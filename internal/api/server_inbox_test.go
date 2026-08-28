package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

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
