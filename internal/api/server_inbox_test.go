package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// hitlResumeFakeRunner updates waiting runs to running on ContinueFromHITL
// and optionally counts resume calls for idempotency assertions.
type hitlResumeFakeRunner struct {
	store store.Store
	n     atomic.Int32
}

func (f *hitlResumeFakeRunner) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	_ = f.store.AppendEvent(runID, store.Event{Type: "run.started"})
	return f.store.UpdateRun(runID, store.StatusSucceeded, "已创建", "")
}

func (f *hitlResumeFakeRunner) ContinueFromHITL(ctx context.Context, runID string, d run.Decision) error {
	f.n.Add(1)
	return f.store.UpdateRun(runID, store.StatusRunning, "", "")
}

func testServerWithInbox(t *testing.T, st store.Store, reg *inbox.Registry) *api.Server {
	t.Helper()
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.Inbox = reg
	srv.InboxLimiter = inbox.NewRateLimiter(inbox.DefaultRateLimit, inbox.DefaultRateWindow)
	return srv
}

func signedInboxRequest(t *testing.T, method, path, secret string, body []byte) *http.Request {
	t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := inbox.Sign(secret, ts, body)
	req := httptest.NewRequest(method, path, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Inbox-Timestamp", ts)
	req.Header.Set("X-Baize-Inbox-Signature", sig)
	return req
}

func TestInboxPostRequiresSignature(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	srv := testServerWithInbox(t, st, reg)
	req := httptest.NewRequest(http.MethodPost, "/v0/inbox/alerts",
		strings.NewReader(`{"input":"hello"}`))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestInboxPostCreatesRun(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	srv := testServerWithInbox(t, st, reg)
	h := srv.Handler()

	body := []byte(`{"input":"hello"}`)
	req := signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}

	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)
	deliveryID, _ := created["delivery_id"].(string)
	if runID == "" || deliveryID == "" {
		t.Fatalf("created=%v", created)
	}
	if !strings.HasPrefix(deliveryID, "dlv_") {
		t.Fatalf("delivery_id=%q", deliveryID)
	}
	if created["status"] != "accepted" {
		t.Fatalf("status=%v", created["status"])
	}

	getEvs := httptest.NewRequest(http.MethodGet, "/v0/runs/"+runID+"/events", nil)
	evRR := httptest.NewRecorder()
	h.ServeHTTP(evRR, getEvs)
	if evRR.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", evRR.Code, evRR.Body.String())
	}
	var evs []store.Event
	if err := json.NewDecoder(evRR.Body).Decode(&evs); err != nil {
		t.Fatal(err)
	}
	if len(evs) < 2 {
		t.Fatalf("events=%+v", evs)
	}
	if evs[0].Type != run.EventInboxReceived {
		t.Fatalf("first event=%q want %q", evs[0].Type, run.EventInboxReceived)
	}
	foundStarted := false
	for _, ev := range evs {
		if ev.Type == run.EventRunStarted {
			foundStarted = true
			break
		}
	}
	if !foundStarted {
		t.Fatalf("events=%+v missing run.started", evs)
	}
}

func seedInboxChannels(t *testing.T, st store.Store, reg *inbox.Registry, channels []inbox.Channel) {
	t.Helper()
	raw, err := json.Marshal(channels)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(store.SettingKeyInboxChannels, raw); err != nil {
		t.Fatal(err)
	}
	reg.Replace(channels)
}

func TestPutInboxChannelsPreservesSecret(t *testing.T) {
	const secret = "my-secret-abcdefghij"
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	seedInboxChannels(t, st, reg, []inbox.Channel{{
		ID: "alerts", AgentID: "a", Secret: secret, Enabled: true,
	}})
	srv := testServerWithInbox(t, st, reg)
	h := srv.Handler()

	putBody := map[string]any{
		"channels": []map[string]any{{
			"id":       "alerts",
			"agent_id": "a",
			"enabled":  true,
		}},
	}
	putReq := httptest.NewRequest(http.MethodPut, "/v0/settings/inbox-channels", jsonBody(t, putBody))
	putRR := httptest.NewRecorder()
	h.ServeHTTP(putRR, putReq)
	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT code=%d body=%s", putRR.Code, putRR.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v0/settings/inbox-channels", nil)
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET code=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var got struct {
		Channels []struct {
			SecretHint string `json:"secret_hint"`
		} `json:"channels"`
	}
	if err := json.NewDecoder(getRR.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Channels) != 1 {
		t.Fatalf("channels=%+v", got.Channels)
	}
	wantHint := inbox.SecretHint(secret)
	if got.Channels[0].SecretHint != wantHint {
		t.Fatalf("secret_hint=%q want %q", got.Channels[0].SecretHint, wantHint)
	}

	body := []byte(`{"input":"hello"}`)
	req := signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", secret, body)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("inbox code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRotateInboxSecret(t *testing.T) {
	const oldSecret = "old-secret-abcdefghij"
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	seedInboxChannels(t, st, reg, []inbox.Channel{{
		ID: "alerts", AgentID: "a", Secret: oldSecret, Enabled: true,
	}})
	srv := testServerWithInbox(t, st, reg)
	h := srv.Handler()

	body := []byte(`{"input":"hello"}`)
	oldReq := signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", oldSecret, body)
	oldRR := httptest.NewRecorder()
	h.ServeHTTP(oldRR, oldReq)
	if oldRR.Code != http.StatusAccepted {
		t.Fatalf("old secret code=%d body=%s", oldRR.Code, oldRR.Body.String())
	}

	rotateReq := httptest.NewRequest(http.MethodPost, "/v0/settings/inbox-channels/alerts/rotate-secret", nil)
	rotateRR := httptest.NewRecorder()
	h.ServeHTTP(rotateRR, rotateReq)
	if rotateRR.Code != http.StatusOK {
		t.Fatalf("rotate code=%d body=%s", rotateRR.Code, rotateRR.Body.String())
	}
	var rotated struct {
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(rotateRR.Body).Decode(&rotated); err != nil {
		t.Fatal(err)
	}
	if rotated.Secret == "" || rotated.Secret == oldSecret {
		t.Fatalf("rotated secret=%q", rotated.Secret)
	}

	failReq := signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", oldSecret, body)
	failRR := httptest.NewRecorder()
	h.ServeHTTP(failRR, failReq)
	if failRR.Code != http.StatusUnauthorized {
		t.Fatalf("old secret after rotate code=%d body=%s", failRR.Code, failRR.Body.String())
	}

	newReq := signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", rotated.Secret, body)
	newRR := httptest.NewRecorder()
	h.ServeHTTP(newRR, newReq)
	if newRR.Code != http.StatusAccepted {
		t.Fatalf("new secret code=%d body=%s", newRR.Code, newRR.Body.String())
	}
}

func decodeInboxErrCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatalf("decode error body: %v raw=%s", err, rr.Body.String())
	}
	return wrap.Error.Code
}

func TestInboxIdempotencyConflict(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	srv := testServerWithInbox(t, st, reg)
	h := srv.Handler()

	body1 := []byte(`{"input":"one","idempotency_key":"same-key"}`)
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body1))
	if rr1.Code != http.StatusAccepted {
		t.Fatalf("first code=%d body=%s", rr1.Code, rr1.Body.String())
	}

	body2 := []byte(`{"input":"two","idempotency_key":"same-key"}`)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body2))
	if rr2.Code != http.StatusConflict {
		t.Fatalf("conflict code=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if got := decodeInboxErrCode(t, rr2); got != "idempotency_conflict" {
		t.Fatalf("code=%q", got)
	}
}

func TestInboxTimestampSkew(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	srv := testServerWithInbox(t, st, reg)

	body := []byte(`{"input":"hello"}`)
	ts := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	sig := inbox.Sign("sec", ts, body)
	req := httptest.NewRequest(http.MethodPost, "/v0/inbox/alerts", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Inbox-Timestamp", ts)
	req.Header.Set("X-Baize-Inbox-Signature", sig)

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := decodeInboxErrCode(t, rr); got != "timestamp_skew" {
		t.Fatalf("code=%q", got)
	}
}

func TestInboxChannelDisabled(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: false}})
	srv := testServerWithInbox(t, st, reg)

	body := []byte(`{"input":"hello"}`)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := decodeInboxErrCode(t, rr); got != "channel_disabled" {
		t.Fatalf("code=%q", got)
	}

	rrMissing := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rrMissing, signedInboxRequest(t, http.MethodPost, "/v0/inbox/missing", "sec", body))
	if rrMissing.Code != http.StatusNotFound {
		t.Fatalf("missing code=%d body=%s", rrMissing.Code, rrMissing.Body.String())
	}
	if got := decodeInboxErrCode(t, rrMissing); got != "channel_not_found" {
		t.Fatalf("missing code=%q", got)
	}
}

func TestPutInboxChannelsEnabledDefaultsTrue(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	srv := testServerWithInbox(t, st, reg)
	h := srv.Handler()

	putBody := map[string]any{
		"channels": []map[string]any{{
			"id":       "alerts",
			"agent_id": "a",
			"secret":   "sec-secret-abcdefghij",
		}},
	}
	putRR := httptest.NewRecorder()
	h.ServeHTTP(putRR, httptest.NewRequest(http.MethodPut, "/v0/settings/inbox-channels", jsonBody(t, putBody)))
	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT code=%d body=%s", putRR.Code, putRR.Body.String())
	}
	var got struct {
		Channels []struct {
			Enabled bool `json:"enabled"`
		} `json:"channels"`
	}
	if err := json.NewDecoder(putRR.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Channels) != 1 || !got.Channels[0].Enabled {
		t.Fatalf("channels=%+v want enabled=true", got.Channels)
	}
}

func TestInboxConcurrentIdempotency(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	srv := testServerWithInbox(t, st, reg)
	h := srv.Handler()

	body := []byte(`{"input":"concurrent","idempotency_key":"conc-key-1"}`)
	const n = 8
	type result struct {
		code  int
		runID string
	}
	results := make([]result, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
			var resp map[string]any
			_ = json.NewDecoder(rr.Body).Decode(&resp)
			runID, _ := resp["run_id"].(string)
			results[i] = result{code: rr.Code, runID: runID}
		}(i)
	}
	wg.Wait()

	var runID string
	for i, r := range results {
		if r.code != http.StatusAccepted && r.code != http.StatusOK {
			t.Fatalf("goroutine %d code=%d", i, r.code)
		}
		if r.runID == "" {
			t.Fatalf("goroutine %d missing run_id", i)
		}
		if runID == "" {
			runID = r.runID
		} else if r.runID != runID {
			t.Fatalf("run_id mismatch: %q vs %q", runID, r.runID)
		}
	}
}

func TestInboxRateLimitAfterSignature(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	srv := testServerWithInbox(t, st, reg)
	srv.InboxLimiter = inbox.NewRateLimiter(1, time.Minute)
	h := srv.Handler()

	body := []byte(`{"input":"hello"}`)
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if rr1.Code != http.StatusAccepted {
		t.Fatalf("first code=%d body=%s", rr1.Code, rr1.Body.String())
	}

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second code=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if got := decodeInboxErrCode(t, rr2); got != "rate_limited" {
		t.Fatalf("code=%q", got)
	}

	// Unsigned requests must not consume the budget (already exhausted above would
	// still be 401, and a fresh limiter proves verify-before-limit ordering).
	srv2 := testServerWithInbox(t, st, reg)
	srv2.InboxLimiter = inbox.NewRateLimiter(1, time.Minute)
	bad := httptest.NewRequest(http.MethodPost, "/v0/inbox/alerts", strings.NewReader(string(body)))
	bad.Header.Set("Content-Type", "application/json")
	badRR := httptest.NewRecorder()
	srv2.Handler().ServeHTTP(badRR, bad)
	if badRR.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned code=%d", badRR.Code)
	}
	okRR := httptest.NewRecorder()
	srv2.Handler().ServeHTTP(okRR, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if okRR.Code != http.StatusAccepted {
		t.Fatalf("signed after unsigned code=%d body=%s", okRR.Code, okRR.Body.String())
	}
}

func TestInboxResumeApprove(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	runner := &hitlResumeFakeRunner{store: st}
	srv := api.NewServer(st, tool.NewRegistry(), runner)
	srv.Inbox = reg
	srv.InboxLimiter = inbox.NewRateLimiter(inbox.DefaultRateLimit, inbox.DefaultRateWindow)
	h := srv.Handler()

	runRec, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "need approve"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(runRec.ID, store.StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"action":"resume","run_id":"` + runRec.ID + `","decision":"approve","comment":"lgtm"}`)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["action"] != "resume" {
		t.Fatalf("action=%v", resp["action"])
	}
	if resp["run_id"] != runRec.ID {
		t.Fatalf("run_id=%v want %s", resp["run_id"], runRec.ID)
	}
	if resp["delivery_id"] == nil || resp["delivery_id"] == "" {
		t.Fatalf("missing delivery_id: %v", resp)
	}

	evs, err := st.ListEvents(runRec.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range evs {
		if ev.Type == run.EventInboxResumed {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events=%+v missing %q", evs, run.EventInboxResumed)
	}
}

func TestInboxResumeForbiddenAgent(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	st.UpsertAgent(store.Agent{ID: "b", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	srv := testServerWithInbox(t, st, reg)
	h := srv.Handler()

	runRec, err := st.CreateRun(store.CreateRunInput{AgentID: "b", Input: "other agent"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(runRec.ID, store.StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"action":"resume","run_id":"` + runRec.ID + `","decision":"approve"}`)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := decodeInboxErrCode(t, rr); got != "run_forbidden" {
		t.Fatalf("code=%q", got)
	}
}

func TestInboxResumeNotWaiting(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	srv := testServerWithInbox(t, st, reg)
	h := srv.Handler()

	runRec, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "done"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(runRec.ID, store.StatusSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"action":"resume","run_id":"` + runRec.ID + `","decision":"approve"}`)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if rr.Code != http.StatusConflict {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := decodeInboxErrCode(t, rr); got != "not_waiting" {
		t.Fatalf("code=%q", got)
	}
}

func TestInboxResumeIdempotent(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	runner := &hitlResumeFakeRunner{store: st}
	srv := api.NewServer(st, tool.NewRegistry(), runner)
	srv.Inbox = reg
	srv.InboxLimiter = inbox.NewRateLimiter(inbox.DefaultRateLimit, inbox.DefaultRateWindow)
	h := srv.Handler()

	runRec, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "need approve"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(runRec.ID, store.StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"action":"resume","run_id":"` + runRec.ID + `","decision":"approve","idempotency_key":"resume-once"}`)
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if rr1.Code != http.StatusOK {
		t.Fatalf("first code=%d body=%s", rr1.Code, rr1.Body.String())
	}
	if n := runner.n.Load(); n != 1 {
		t.Fatalf("ContinueFromHITL calls after first=%d want 1", n)
	}

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if rr2.Code != http.StatusOK {
		t.Fatalf("second code=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if n := runner.n.Load(); n != 1 {
		t.Fatalf("ContinueFromHITL calls after replay=%d want 1", n)
	}
	var resp map[string]any
	if err := json.NewDecoder(rr2.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["action"] != "resume" || resp["run_id"] != runRec.ID {
		t.Fatalf("replay resp=%v", resp)
	}
}

func TestInboxResumeNotWaitingWithIdempotencyKey(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	srv := testServerWithInbox(t, st, reg)
	h := srv.Handler()

	runRec, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "done"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(runRec.ID, store.StatusSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"action":"resume","run_id":"` + runRec.ID + `","decision":"approve","idempotency_key":"resume-fail-key"}`)
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if rr1.Code != http.StatusConflict {
		t.Fatalf("first code=%d body=%s", rr1.Code, rr1.Body.String())
	}
	if got := decodeInboxErrCode(t, rr1); got != "not_waiting" {
		t.Fatalf("first code=%q", got)
	}

	// Same key + body must still return the business error, not a wait timeout 500
	// from a poisoned empty-RunID delivery slot.
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, signedInboxRequest(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if rr2.Code != http.StatusConflict {
		t.Fatalf("replay code=%d body=%s", rr2.Code, rr2.Body.String())
	}
	if got := decodeInboxErrCode(t, rr2); got != "not_waiting" {
		t.Fatalf("replay code=%q", got)
	}
}
