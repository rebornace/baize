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
