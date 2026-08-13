package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestPostRunPassthroughHiddenFromGetRun(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	srv.AuthMode = "passthrough"
	h := srv.Handler()

	st.UpsertAgent(store.Agent{ID: "a", System: "s"})

	req := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "a", "input": "hi"}))
	req.Header.Set("Authorization", "Bearer HIDDEN_TOKEN")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)
	if runID == "" {
		t.Fatalf("created=%v", created)
	}

	got, _ := st.GetRun(runID)
	if got.PassthroughHeaders["Authorization"] != "Bearer HIDDEN_TOKEN" {
		t.Fatalf("store passthrough=%+v", got.PassthroughHeaders)
	}

	get := httptest.NewRequest(http.MethodGet, "/v0/runs/"+runID, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get run status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "HIDDEN_TOKEN") {
		t.Fatalf("GET run leaked token: %s", rr.Body.String())
	}

	getEvs := httptest.NewRequest(http.MethodGet, "/v0/runs/"+runID+"/events", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getEvs)
	if rr.Code != http.StatusOK {
		t.Fatalf("get events status=%d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "HIDDEN_TOKEN") {
		t.Fatalf("GET events leaked token: %s", rr.Body.String())
	}
}

func TestPostRunNonPassthroughDoesNotPickHeaders(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	st.UpsertAgent(store.Agent{ID: "a", System: "s"})

	req := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "a", "input": "hi"}))
	req.Header.Set("Authorization", "Bearer SHOULD_NOT_BE_STORED")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)
	runID, _ := created["run_id"].(string)
	got, _ := st.GetRun(runID)
	if len(got.PassthroughHeaders) != 0 {
		t.Fatalf("non-passthrough mode picked headers: %+v", got.PassthroughHeaders)
	}
}

func TestPostRunPassthroughExplicitWhitelist(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	srv.AuthMode = "passthrough"
	srv.AuthWhitelist = []string{"X-Custom-Auth"}
	h := srv.Handler()

	st.UpsertAgent(store.Agent{ID: "a", System: "s"})

	req := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "a", "input": "hi"}))
	req.Header.Set("Authorization", "Bearer IGNORED")
	req.Header.Set("X-Custom-Auth", "token-custom")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)
	runID, _ := created["run_id"].(string)
	got, _ := st.GetRun(runID)
	if got.PassthroughHeaders["X-Custom-Auth"] != "token-custom" {
		t.Fatalf("missing whitelisted header: %+v", got.PassthroughHeaders)
	}
	if _, ok := got.PassthroughHeaders["Authorization"]; ok {
		t.Fatalf("Authorization should not be picked with explicit whitelist: %+v", got.PassthroughHeaders)
	}
}

type capturingRunner struct {
	store        store.Store
	captured     map[string]string
	capturedOnce bool
}

func (c *capturingRunner) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	return c.store.UpdateRun(runID, store.StatusWaitingHuman, "", "")
}

func (c *capturingRunner) ContinueFromHITL(ctx context.Context, runID string, d run.Decision) error {
	if r, err := c.store.GetRun(runID); err == nil {
		c.captured = r.PassthroughHeaders
	}
	c.capturedOnce = true
	return c.store.UpdateRun(runID, store.StatusSucceeded, "resumed", "")
}

func waitForStatus(t *testing.T, st store.Store, runID string, want store.Status) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r, err := st.GetRun(runID)
		if err == nil && r.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	r, _ := st.GetRun(runID)
	t.Fatalf("run not %s: %+v", want, r)
}

func TestPostResumePassthroughSetsHeadersBeforeContinue(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	runner := &capturingRunner{store: st}
	srv := api.NewServer(st, reg, runner)
	srv.AuthMode = "passthrough"
	h := srv.Handler()

	st.UpsertAgent(store.Agent{ID: "a", System: "s"})

	postRun := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "a", "input": "hi"}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, postRun)
	if rr.Code != http.StatusOK {
		t.Fatalf("post status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)
	runID, _ := created["run_id"].(string)

	waitForStatus(t, st, runID, store.StatusWaitingHuman)

	resume := httptest.NewRequest(http.MethodPost, "/v0/runs/"+runID+"/resume",
		jsonBody(t, map[string]any{"decision": "approve", "comment": "ok"}))
	resume.Header.Set("Authorization", "Bearer RESUME_TOKEN")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, resume)
	if rr.Code != http.StatusOK {
		t.Fatalf("resume status=%d body=%s", rr.Code, rr.Body.String())
	}

	if !runner.capturedOnce {
		t.Fatal("ContinueFromHITL was not invoked")
	}
	if runner.captured["Authorization"] != "Bearer RESUME_TOKEN" {
		t.Fatalf("resume did not set passthrough headers before continue; captured=%+v", runner.captured)
	}
}

func TestPostResumeNonPassthroughDoesNotSetHeaders(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	runner := &capturingRunner{store: st}
	srv := api.NewServer(st, reg, runner)
	h := srv.Handler()

	st.UpsertAgent(store.Agent{ID: "a", System: "s"})

	postRun := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "a", "input": "hi"}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, postRun)
	if rr.Code != http.StatusOK {
		t.Fatalf("post status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)
	runID, _ := created["run_id"].(string)

	waitForStatus(t, st, runID, store.StatusWaitingHuman)

	resume := httptest.NewRequest(http.MethodPost, "/v0/runs/"+runID+"/resume",
		jsonBody(t, map[string]any{"decision": "approve"}))
	resume.Header.Set("Authorization", "Bearer SHOULD_NOT_BE_STORED")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, resume)
	if rr.Code != http.StatusOK {
		t.Fatalf("resume status=%d body=%s", rr.Code, rr.Body.String())
	}

	if !runner.capturedOnce {
		t.Fatal("ContinueFromHITL was not invoked")
	}
	if len(runner.captured) != 0 {
		t.Fatalf("non-passthrough resume set headers: %+v", runner.captured)
	}
}
