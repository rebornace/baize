package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
)

// Real LLM eval runs are not part of default CI. For manual integration:
// go test ./internal/api/ -run TestSkillEvalRunLive -count=1
// (skipped when testing.Short() is set).

func evidenceEvalDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "scripts", "evidence-eval"))
}

func TestScoreSkillEvalCaseFromEvents(t *testing.T) {
	dir := evidenceEvalDir(t)
	rawCases, err := os.ReadFile(filepath.Join(dir, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []skill.EvalCase
	if err := json.Unmarshal(rawCases, &cases); err != nil {
		t.Fatal(err)
	}
	var evalCase skill.EvalCase
	for _, c := range cases {
		if c.ID == "EV-V0-list-only" {
			evalCase = c
			break
		}
	}
	if evalCase.ID == "" {
		t.Fatal("EV-V0-list-only not found in cases.json")
	}

	evPath := filepath.Join(dir, "fixtures", "EV-V0-list-only.pass.events.json")
	rawEvents, err := os.ReadFile(evPath)
	if err != nil {
		t.Fatal(err)
	}
	var evs []store.Event
	if err := json.Unmarshal(rawEvents, &evs); err != nil {
		t.Fatal(err)
	}

	res := skill.ScoreEvalCaseFromRun(evalCase, evs, string(store.StatusSucceeded))
	if !res.Pass {
		t.Fatalf("expected pass, got codes=%v notes=%s", res.Codes, res.Notes)
	}
}

func TestSkillEvalRunReturnsScore(t *testing.T) {
	dir := evidenceEvalDir(t)
	rawEvents, err := os.ReadFile(filepath.Join(dir, "fixtures", "EV-V0-list-only.pass.events.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtureEvents []store.Event
	if err := json.Unmarshal(rawEvents, &fixtureEvents); err != nil {
		t.Fatal(err)
	}

	srv, st, h, _ := skillsServer(t)
	st.UpsertAgent(store.Agent{ID: "eval-agent", System: "test"})
	srv.Runner = &evalFixtureRunner{store: st, events: fixtureEvents}

	body, ctype := multipartFile(t, "file", "demo.md", skillMDBytes("demo", "user-demo", []string{"t1"}))
	post := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	post.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST skill status=%d body=%s", rr.Code, rr.Body.String())
	}

	evalBody := `{
		"harness": "evidence-eval",
		"cases": [{
			"id": "EV-V0-list-only",
			"protocol": "V0",
			"input": "list only",
			"required_reads": ["list_tickets"],
			"consequential": ["create_ticket", "update_ticket_status"]
		}]
	}`
	put := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/eval", strings.NewReader(evalBody))
	put.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT eval status=%d body=%s", rr.Code, rr.Body.String())
	}

	runReq := httptest.NewRequest(http.MethodPost, "/v0/skills/demo/eval/run",
		strings.NewReader(`{"agent_id":"eval-agent","case_ids":["EV-V0-list-only"]}`))
	runReq.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, runReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST eval/run status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Results []skillEvalRunCaseResult `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results=%+v", out.Results)
	}
	r := out.Results[0]
	if r.ID != "EV-V0-list-only" || !r.Pass || r.RunID == "" {
		t.Fatalf("result=%+v", r)
	}
}

type evalFixtureRunner struct {
	store  store.Store
	events []store.Event
}

func (f *evalFixtureRunner) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	for _, ev := range f.events {
		if ev.Type == "run.started" {
			continue
		}
		_ = f.store.AppendEvent(runID, ev)
	}
	return f.store.UpdateRun(runID, store.StatusSucceeded, "done", "")
}

func (f *evalFixtureRunner) ContinueFromHITL(ctx context.Context, runID string, d run.Decision) error {
	return nil
}

type slowEvalRunner struct {
	store store.Store
	start chan struct{}
	done  chan struct{}
}

func (r *slowEvalRunner) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	close(r.start)
	select {
	case <-r.done:
		return r.store.UpdateRun(runID, store.StatusSucceeded, "", "")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *slowEvalRunner) ContinueFromHITL(ctx context.Context, runID string, d run.Decision) error {
	return nil
}

func TestSkillEvalRunBusyConflict(t *testing.T) {
	srv, st, h, _ := skillsServer(t)
	st.UpsertAgent(store.Agent{ID: "eval-agent", System: "test"})
	slow := &slowEvalRunner{store: st, start: make(chan struct{}), done: make(chan struct{})}
	srv.Runner = slow

	body, ctype := multipartFile(t, "file", "demo.md", skillMDBytes("demo", "x", nil))
	post := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	post.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	put := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/eval", strings.NewReader(validEvalJSON()))
	put.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req := httptest.NewRequest(http.MethodPost, "/v0/skills/demo/eval/run",
			strings.NewReader(`{"agent_id":"eval-agent","case_ids":["c1"]}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("first run status=%d body=%s", rr.Code, rr.Body.String())
		}
	}()

	select {
	case <-slow.start:
	case <-time.After(2 * time.Second):
		t.Fatal("slow runner never started")
	}

	req := httptest.NewRequest(http.MethodPost, "/v0/skills/demo/eval/run",
		strings.NewReader(`{"agent_id":"eval-agent","case_ids":["c1"]}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("second run status=%d body=%s", rr.Code, rr.Body.String())
	}

	close(slow.done)
	wg.Wait()
}

func TestSkillEvalRunRequiresAdmin(t *testing.T) {
	srv, st, h, cat := skillsServer(t)
	srv.OperatorToken = "op"
	srv.AdminToken = "adm"
	srv.SkillCatalog = cat
	st.UpsertAgent(store.Agent{ID: "eval-agent", System: "test"})

	body, ctype := multipartFile(t, "file", "demo.md", skillMDBytes("demo", "x", nil))
	post := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	post.Header.Set("Content-Type", ctype)
	post.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	put := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/eval", strings.NewReader(validEvalJSON()))
	put.Header.Set("Content-Type", "application/json")
	put.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/v0/skills/demo/eval/run",
		strings.NewReader(`{"agent_id":"eval-agent"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer op")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operator run want 403, got %d %s", rr.Code, rr.Body.String())
	}
}

func TestSkillEvalRunWaitingHumanScoresAndStops(t *testing.T) {
	srv, st, h, _ := skillsServer(t)
	st.UpsertAgent(store.Agent{ID: "eval-agent", System: "test"})
	srv.Runner = &waitingHumanRunner{store: st}

	body, ctype := multipartFile(t, "file", "demo.md", skillMDBytes("demo", "x", nil))
	post := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	post.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	evalBody := `{
		"harness": "evidence-eval",
		"cases": [{
			"id": "hitl-case",
			"protocol": "V1",
			"input": "update",
			"required_reads": ["get_ticket"],
			"consequential": ["update_ticket_status"],
			"expect_consequential_any": ["update_ticket_status"]
		}]
	}`
	put := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/eval", strings.NewReader(evalBody))
	put.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/v0/skills/demo/eval/run",
		strings.NewReader(`{"agent_id":"eval-agent","case_ids":["hitl-case"]}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Results []skillEvalRunCaseResult `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results=%+v", out.Results)
	}
	got := out.Results[0]
	if got.RunID == "" {
		t.Fatal("missing run_id")
	}
	runRec, err := st.GetRun(got.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if runRec.Status != store.StatusWaitingHuman {
		t.Fatalf("status=%s want waiting_human", runRec.Status)
	}
	if !got.Pass {
		t.Fatalf("expected pass after HITL trajectory score, codes=%v notes=%s", got.Codes, got.Notes)
	}
	if len(got.Codes) != 0 {
		t.Fatalf("expected no failure codes, got %v", got.Codes)
	}
}

func TestSkillEvalRunPollTimeoutAddsTimeoutCode(t *testing.T) {
	srv, st, h, _ := skillsServer(t)
	st.UpsertAgent(store.Agent{ID: "eval-agent", System: "test"})
	srv.skillEvalRunPollTimeout = 80 * time.Millisecond
	stall := make(chan struct{})
	srv.Runner = &stallEvalRunner{stall: stall}

	body, ctype := multipartFile(t, "file", "demo.md", skillMDBytes("demo", "x", nil))
	post := httptest.NewRequest(http.MethodPost, "/v0/skills", body)
	post.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	put := httptest.NewRequest(http.MethodPut, "/v0/skills/demo/eval", strings.NewReader(validEvalJSON()))
	put.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/v0/skills/demo/eval/run",
		strings.NewReader(`{"agent_id":"eval-agent","case_ids":["c1"]}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Results []skillEvalRunCaseResult `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 1 {
		t.Fatalf("results=%+v", out.Results)
	}
	got := out.Results[0]
	if got.Pass {
		t.Fatalf("expected fail on timeout, result=%+v", got)
	}
	hasTimeout := false
	for _, c := range got.Codes {
		if c == "TIMEOUT" {
			hasTimeout = true
			break
		}
	}
	if !hasTimeout {
		t.Fatalf("codes=%v want TIMEOUT", got.Codes)
	}
	close(stall)
}

type stallEvalRunner struct {
	stall chan struct{}
}

func (r *stallEvalRunner) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	select {
	case <-r.stall:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *stallEvalRunner) ContinueFromHITL(ctx context.Context, runID string, d run.Decision) error {
	return nil
}

type waitingHumanRunner struct {
	store store.Store
}

func (w *waitingHumanRunner) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	_ = w.store.AppendEvent(runID, store.Event{
		Type: "llm.tool_call",
		Data: map[string]any{"id": "c1", "name": "get_ticket", "arguments": map[string]any{"id": "demo-1"}},
	})
	_ = w.store.AppendEvent(runID, store.Event{
		Type: "tool.result",
		Data: map[string]any{"id": "c1", "is_error": false},
	})
	_ = w.store.AppendEvent(runID, store.Event{
		Type: "llm.tool_call",
		Data: map[string]any{"id": "c2", "name": "update_ticket_status", "arguments": map[string]any{"id": "demo-1", "status": "closed"}},
	})
	_ = w.store.AppendEvent(runID, store.Event{Type: "hitl.waiting", Data: map[string]any{"tool_name": "update_ticket_status"}})
	return w.store.UpdateRun(runID, store.StatusWaitingHuman, "", "")
}

func (w *waitingHumanRunner) ContinueFromHITL(ctx context.Context, runID string, d run.Decision) error {
	return nil
}
