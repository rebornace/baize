package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/plugincallback"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func signedInboxReq(t *testing.T, method, path, secret string, body []byte) *http.Request {
	t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := inbox.Sign(secret, ts, body)
	req := httptest.NewRequest(method, path, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Inbox-Timestamp", ts)
	req.Header.Set("X-Baize-Inbox-Signature", sig)
	return req
}

func TestInboxGateRejectsWhenFalse(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.Inbox = reg
	srv.InboxLimiter = inbox.NewRateLimiter(inbox.DefaultRateLimit, inbox.DefaultRateWindow)

	var calls atomic.Int32
	srv.InboxGate = func(channelID string) bool {
		calls.Add(1)
		if channelID != "alerts" {
			t.Errorf("channelID=%q", channelID)
		}
		return false
	}

	body := []byte(`{"input":"hello"}`)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, signedInboxReq(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("code=%d want 429 body=%s", rr.Code, rr.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("InboxGate calls=%d want 1", calls.Load())
	}
}

func TestInboxGateNilFallsBackToLimiter(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.Inbox = reg
	srv.InboxLimiter = inbox.NewRateLimiter(1, time.Minute)
	srv.InboxGate = nil

	body := []byte(`{"input":"hello"}`)
	h := srv.Handler()
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, signedInboxReq(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if rr1.Code != http.StatusAccepted {
		t.Fatalf("first code=%d body=%s", rr1.Code, rr1.Body.String())
	}
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, signedInboxReq(t, http.MethodPost, "/v0/inbox/alerts", "sec", body))
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second code=%d want 429 body=%s", rr2.Code, rr2.Body.String())
	}
}

func TestCallbackGateRejectsWhenFalse(t *testing.T) {
	srv, _, h, runID, secret := callbackTestServer(t)
	var calls atomic.Int32
	srv.CallbackGate = func(id string) bool {
		calls.Add(1)
		if id != runID {
			t.Errorf("runID=%q want %q", id, runID)
		}
		return false
	}
	token, _, _ := plugincallback.Issue(secret, runID, time.Hour)
	req := httptest.NewRequest(http.MethodPost, "/v0/runs/"+runID+"/plugin-callbacks?token="+token, strings.NewReader(`{"type":"x","name":"echo"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d want 429 body=%s", rr.Code, rr.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("CallbackGate calls=%d want 1", calls.Load())
	}
}

func TestCallbackGateNilFallsBackToLimiter(t *testing.T) {
	srv, _, h, runID, secret := callbackTestServer(t)
	srv.CallbackGate = nil
	srv.CallbackLimiter = plugincallback.NewLimiter(1, time.Hour)
	token, _, _ := plugincallback.Issue(secret, runID, time.Hour)
	body := `{"type":"x","name":"echo"}`
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v0/runs/"+runID+"/plugin-callbacks?token="+token, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}
	if rr := post(); rr.Code != http.StatusNoContent {
		t.Fatalf("first status=%d want 204 body=%s", rr.Code, rr.Body.String())
	}
	if rr := post(); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("second status=%d want 429 body=%s", rr.Code, rr.Body.String())
	}
}
