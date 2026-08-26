package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/plugincallback"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// callbackTestServer builds a Server wired with a callback secret and a
// fresh per-run limiter, plus a memory store holding one run (run_cb).
func callbackTestServer(t *testing.T) (*Server, store.Store, http.Handler, string, []byte) {
	t.Helper()
	st := store.NewMemory()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	secret := []byte("topsecret")
	srv.CallbackSecret = secret
	srv.CallbackLimiter = plugincallback.NewLimiter(plugincallback.DefaultBudget, plugincallback.DefaultWindow)
	runRec, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	return srv, st, srv.Handler(), runRec.ID, secret
}

func TestPluginCallbackHappyPath(t *testing.T) {
	_, st, h, runID, secret := callbackTestServer(t)
	token, _, err := plugincallback.Issue(secret, runID, time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	body := map[string]any{
		"type":    "task.completed",
		"name":    "echo",
		"payload": map[string]any{"ok": true, "msg": "hi"},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v0/runs/"+runID+"/plugin-callbacks?token="+token, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204 body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("204 must have empty body, got %q", rr.Body.String())
	}
	evs, err := st.ListEvents(runID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	var found *store.Event
	for i := range evs {
		if evs[i].Type == "plugin.callback" {
			found = &evs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no plugin.callback event among %d events", len(evs))
	}
	if got := found.Data["name"]; got != "echo" {
		t.Fatalf("event.data.name=%v want echo", got)
	}
	if payload, ok := found.Data["payload"].(map[string]any); !ok || payload["ok"] != true {
		t.Fatalf("event.data.payload mismatch: %+v", found.Data["payload"])
	}
	// The sidecar's body.type is preserved as data.type (Event.Type is fixed).
	if got := found.Data["type"]; got != "task.completed" {
		t.Fatalf("event.data.type=%v want task.completed", got)
	}
}

func TestPluginCallbackBadToken(t *testing.T) {
	srv, st, h, runID, _ := callbackTestServer(t)
	_ = srv
	_ = st
	// wrong token
	req := httptest.NewRequest(http.MethodPost, "/v0/runs/"+runID+"/plugin-callbacks?token=not-a-real-token", strings.NewReader(`{"type":"x","name":"echo"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("bad token status=%d want 401 body=%s", rr.Code, rr.Body.String())
	}
	// missing token
	req2 := httptest.NewRequest(http.MethodPost, "/v0/runs/"+runID+"/plugin-callbacks", strings.NewReader(`{"type":"x","name":"echo"}`))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status=%d want 401 body=%s", rr2.Code, rr2.Body.String())
	}
	// token bound to a different run must fail (Verify rebinds to path runID)
	token, _, _ := plugincallback.Issue([]byte("topsecret"), "run_other", time.Hour)
	req3 := httptest.NewRequest(http.MethodPost, "/v0/runs/"+runID+"/plugin-callbacks?token="+token, strings.NewReader(`{"type":"x","name":"echo"}`))
	req3.Header.Set("Content-Type", "application/json")
	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusUnauthorized {
		t.Fatalf("cross-run token status=%d want 401 body=%s", rr3.Code, rr3.Body.String())
	}
}

func TestPluginCallbackRunNotFound(t *testing.T) {
	srv, _, h, _, secret := callbackTestServer(t)
	_ = srv
	token, _, _ := plugincallback.Issue(secret, "run_missing", time.Hour)
	req := httptest.NewRequest(http.MethodPost, "/v0/runs/run_missing/plugin-callbacks?token="+token, strings.NewReader(`{"type":"x","name":"echo"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing run status=%d want 404 body=%s", rr.Code, rr.Body.String())
	}
}

func TestPluginCallbackRateLimited(t *testing.T) {
	srv, st, h, runID, secret := callbackTestServer(t)
	_ = st
	// Replace limiter with budget=1 so the second call is rejected.
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
		t.Fatalf("first call status=%d want 204 body=%s", rr.Code, rr.Body.String())
	}
	if rr := post(); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("second call status=%d want 429 body=%s", rr.Code, rr.Body.String())
	}
}

func TestPluginCallbackOversizedPayload(t *testing.T) {
	srv, _, h, runID, secret := callbackTestServer(t)
	_ = srv
	token, _, _ := plugincallback.Issue(secret, runID, time.Hour)
	// Build a payload whose JSON serialization exceeds 64KiB.
	big := strings.Repeat("x", 70*1024)
	body := map[string]any{
		"type":    "big",
		"name":    "echo",
		"payload": map[string]any{"blob": big},
	}
	raw, _ := json.Marshal(body)
	if len(raw) <= 64*1024 {
		t.Fatalf("test payload too small: %d bytes", len(raw))
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/runs/"+runID+"/plugin-callbacks?token="+token, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized payload status=%d want 413 body=%s", rr.Code, rr.Body.String())
	}
}

func TestPluginCallbackGateOffAllowsAnonymous(t *testing.T) {
	// Even with the control-plane gate on, plugin-callbacks is RoleNone so
	// no Bearer token is required; only the HMAC token matters.
	srv, st, h, runID, secret := callbackTestServer(t)
	srv.OperatorToken = "op"
	srv.AdminToken = "adm"
	_ = st
	token, _, _ := plugincallback.Issue(secret, runID, time.Hour)
	body := `{"type":"x","name":"echo","payload":{"ok":true}}`
	req := httptest.NewRequest(http.MethodPost, "/v0/runs/"+runID+"/plugin-callbacks?token="+token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("gate-on anonymous callback status=%d want 204 body=%s", rr.Code, rr.Body.String())
	}
}
