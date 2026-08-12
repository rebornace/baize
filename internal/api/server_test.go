package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type fakeRunner struct {
	store store.Store
}

func (f *fakeRunner) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	_ = f.store.AppendEvent(runID, store.Event{Type: "run.started"})
	return f.store.UpdateRun(runID, store.StatusSucceeded, "已创建", "")
}

func (f *fakeRunner) ContinueFromHITL(ctx context.Context, runID string, d run.Decision) error {
	return nil
}

type scriptLLM struct{ calls int }

func (s *scriptLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	s.calls++
	if s.calls == 1 {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "create_ticket", Arguments: map[string]any{"title": "x"}},
		}}, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: "已创建"}, nil
}

func TestUIIndex(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Baize Chat") {
		t.Fatalf("body=%q", rr.Body.String())
	}
}

func TestHealthz(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body=%v", body)
	}
}

func TestRunSucceedsWithFakeRunner(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	putAgent := httptest.NewRequest(http.MethodPut, "/v0/agents/ticket-agent",
		jsonBody(t, map[string]any{"system": "你是工单助手"}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, putAgent)
	if rr.Code != http.StatusOK {
		t.Fatalf("put agent status=%d body=%s", rr.Code, rr.Body.String())
	}

	postRun := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "ticket-agent", "input": "创建工单：测试"}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, postRun)
	if rr.Code != http.StatusOK {
		t.Fatalf("post run status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)
	if runID == "" {
		t.Fatalf("created=%v", created)
	}

	runRec := pollRunStatus(t, h, runID, store.StatusSucceeded)
	if runRec.Output != "已创建" {
		t.Fatalf("run=%+v", runRec)
	}

	getEvs := httptest.NewRequest(http.MethodGet, "/v0/runs/"+runID+"/events", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getEvs)
	if rr.Code != http.StatusOK {
		t.Fatalf("events status=%d", rr.Code)
	}
	var evs []store.Event
	if err := json.NewDecoder(rr.Body).Decode(&evs); err != nil {
		t.Fatal(err)
	}
	if len(evs) == 0 || evs[0].Type != "run.started" {
		t.Fatalf("events=%+v", evs)
	}
}

func TestUnknownAgent(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})

	req := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "missing", "input": "x"}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
	var wrap struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code == "" || wrap.Error.Message == "" {
		t.Fatalf("error body=%+v", wrap)
	}
}

func TestPutConnectorRegistersTools(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "openapi.yaml")
	content := []byte(`openapi: 3.0.3
info:
  title: t
  version: 0.1.0
paths:
  /tickets:
    post:
      operationId: create_ticket
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                title: { type: string }
      responses:
        "201":
          description: created
`)
	if err := os.WriteFile(specPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})

	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":              "openapi",
			"spec":              specPath,
			"base_url":          "http://127.0.0.1:18080",
			"require_approval":  []string{"create_ticket"},
		}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	specs := reg.Specs()
	if len(specs) != 1 || specs[0].Name != "create_ticket" {
		t.Fatalf("specs=%+v", specs)
	}
	if !reg.RequiresApproval("create_ticket") {
		t.Fatal("create_ticket should require approval")
	}
	c, err := st.GetConnector("ticket")
	if err != nil || c.BaseURL != "http://127.0.0.1:18080" {
		t.Fatalf("connector=%+v err=%v", c, err)
	}
}

func TestGetToolsAndConnector(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "openapi.yaml")
	if err := os.WriteFile(specPath, []byte(`openapi: 3.0.3
info: { title: t, version: 0.1.0 }
paths:
  /tickets:
    post:
      operationId: create_ticket
      responses: { "201": { description: created } }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	put := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":     "openapi",
			"spec":     specPath,
			"base_url": "http://127.0.0.1:18080",
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", rr.Code, rr.Body.String())
	}

	getTools := httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getTools)
	if rr.Code != http.StatusOK {
		t.Fatalf("get tools status=%d", rr.Code)
	}
	var toolsBody struct {
		Tools []tool.Info `json:"tools"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&toolsBody); err != nil {
		t.Fatal(err)
	}
	if len(toolsBody.Tools) == 0 {
		t.Fatalf("expected tools, got %+v", toolsBody)
	}
	found := false
	for _, info := range toolsBody.Tools {
		if info.Name == "create_ticket" {
			found = true
			if info.Method == "" || info.Path == "" {
				t.Fatalf("create_ticket missing method/path: %+v", info)
			}
		}
	}
	if !found {
		t.Fatalf("create_ticket not in tools: %+v", toolsBody.Tools)
	}

	getConn := httptest.NewRequest(http.MethodGet, "/v0/connectors/ticket", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getConn)
	if rr.Code != http.StatusOK {
		t.Fatalf("get connector status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPutConnectorToolConflict(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "openapi.yaml")
	if err := os.WriteFile(specPath, []byte(`openapi: 3.0.3
info: { title: t, version: 0.1.0 }
paths:
  /tickets:
    post:
      operationId: create_ticket
      responses: { "201": { description: created } }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	putA := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket-a",
		jsonBody(t, map[string]any{"type": "openapi", "spec": specPath, "base_url": "http://a"}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, putA)
	if rr.Code != http.StatusOK {
		t.Fatalf("put A status=%d body=%s", rr.Code, rr.Body.String())
	}

	putB := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket-b",
		jsonBody(t, map[string]any{"type": "openapi", "spec": specPath, "base_url": "http://b"}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, putB)
	if rr.Code != http.StatusConflict {
		t.Fatalf("put B status=%d want 409 body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "tool_conflict" {
		t.Fatalf("code=%q", wrap.Error.Code)
	}
}

func TestPutConnectorInvalidSpecLeavesRegistry(t *testing.T) {
	dir := t.TempDir()
	goodSpec := filepath.Join(dir, "good.yaml")
	if err := os.WriteFile(goodSpec, []byte(`openapi: 3.0.3
info: { title: t, version: 0.1.0 }
paths:
  /tickets:
    post:
      operationId: create_ticket
      responses: { "201": { description: created } }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	putOK := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{"type": "openapi", "spec": goodSpec, "base_url": "http://x"}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, putOK)
	if rr.Code != http.StatusOK {
		t.Fatalf("first put status=%d body=%s", rr.Code, rr.Body.String())
	}

	putBad := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{"type": "openapi", "spec": filepath.Join(dir, "missing.yaml"), "base_url": "http://x"}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, putBad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad put status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "invalid_spec" {
		t.Fatalf("code=%q", wrap.Error.Code)
	}

	getTools := httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getTools)
	if rr.Code != http.StatusOK {
		t.Fatalf("get tools status=%d", rr.Code)
	}
	var toolsBody struct {
		Tools []tool.Info `json:"tools"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&toolsBody); err != nil {
		t.Fatal(err)
	}
	if len(toolsBody.Tools) != 1 || toolsBody.Tools[0].Name != "create_ticket" {
		t.Fatalf("tools after invalid put=%+v want first create_ticket", toolsBody.Tools)
	}
}

func TestPutConnectorResponseIncludesTools(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "openapi.yaml")
	if err := os.WriteFile(specPath, []byte(`openapi: 3.0.3
info: { title: t, version: 0.1.0 }
paths:
  /tickets:
    post:
      operationId: create_ticket
      responses: { "201": { description: created } }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})

	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":             "openapi",
			"spec":             specPath,
			"base_url":         "http://x",
			"require_approval": []string{"create_ticket"},
		}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		ID               string      `json:"id"`
		Type             string      `json:"type"`
		Spec             string      `json:"spec"`
		BaseURL          string      `json:"base_url"`
		RequireApproval  []string    `json:"require_approval"`
		Tools            []tool.Info `json:"tools"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ID != "ticket" || body.Type != "openapi" || body.Spec != specPath {
		t.Fatalf("body=%+v", body)
	}
	if len(body.Tools) == 0 {
		t.Fatalf("expected tools array in response: %+v", body)
	}
	if len(body.RequireApproval) != 1 || body.RequireApproval[0] != "create_ticket" {
		t.Fatalf("require_approval=%v", body.RequireApproval)
	}
}

func TestPutConnectorReplacesOldTools(t *testing.T) {
	dir := t.TempDir()
	specA := filepath.Join(dir, "a.yaml")
	specB := filepath.Join(dir, "b.yaml")
	if err := os.WriteFile(specA, []byte(`openapi: 3.0.3
info: { title: t, version: 0.1.0 }
paths:
  /tickets:
    post:
      operationId: create_ticket
      responses: { "201": { description: created } }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specB, []byte(`openapi: 3.0.3
info: { title: t, version: 0.1.0 }
paths:
  /tickets:
    get:
      operationId: list_tickets
      responses: { "200": { description: ok } }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	put := func(spec string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
			jsonBody(t, map[string]any{"type": "openapi", "spec": spec, "base_url": "http://x"}))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	}
	put(specA)
	put(specB)

	specs := reg.Specs()
	if len(specs) != 1 || specs[0].Name != "list_tickets" {
		t.Fatalf("specs=%+v want only list_tickets", specs)
	}
}

func TestResumeApproveSucceeds(t *testing.T) {
	st, reg, eng, h := newHITLServer(t)

	putAgent := httptest.NewRequest(http.MethodPut, "/v0/agents/ticket-agent",
		jsonBody(t, map[string]any{"system": "helper"}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, putAgent)
	if rr.Code != http.StatusOK {
		t.Fatalf("put agent=%d", rr.Code)
	}
	_ = st
	_ = eng

	postRun := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "ticket-agent", "input": "创建工单"}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, postRun)
	if rr.Code != http.StatusOK {
		t.Fatalf("post run=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)

	pollRunStatus(t, h, runID, store.StatusWaitingHuman)
	if !reg.RequiresApproval("create_ticket") {
		t.Fatal("expected require approval")
	}

	resume := httptest.NewRequest(http.MethodPost, "/v0/runs/"+runID+"/resume",
		jsonBody(t, map[string]any{"decision": "approve", "comment": "ok"}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, resume)
	if rr.Code != http.StatusOK {
		t.Fatalf("resume=%d body=%s", rr.Code, rr.Body.String())
	}

	got := pollRunStatus(t, h, runID, store.StatusSucceeded)
	if got.Output != "已创建" {
		t.Fatalf("run=%+v", got)
	}
}

func TestResumeRejectFails(t *testing.T) {
	_, _, _, h := newHITLServer(t)

	putAgent := httptest.NewRequest(http.MethodPut, "/v0/agents/ticket-agent",
		jsonBody(t, map[string]any{"system": "helper"}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, putAgent)

	postRun := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "ticket-agent", "input": "创建工单"}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, postRun)
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)
	runID, _ := created["run_id"].(string)

	pollRunStatus(t, h, runID, store.StatusWaitingHuman)

	resume := httptest.NewRequest(http.MethodPost, "/v0/runs/"+runID+"/resume",
		jsonBody(t, map[string]any{"decision": "reject", "comment": "no"}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, resume)
	if rr.Code != http.StatusOK {
		t.Fatalf("resume=%d body=%s", rr.Code, rr.Body.String())
	}

	pollRunStatus(t, h, runID, store.StatusFailed)
}

func TestResumeNotWaitingConflict(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	st.UpsertAgent(store.Agent{ID: "a", System: "s"})
	r, err := st.CreateRun("a", "x")
	if err != nil {
		t.Fatal(err)
	}
	_ = st.UpdateRun(r.ID, store.StatusSucceeded, "done", "")

	resume := httptest.NewRequest(http.MethodPost, "/v0/runs/"+r.ID+"/resume",
		jsonBody(t, map[string]any{"decision": "approve"}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, resume)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409", rr.Code)
	}
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "not_waiting" {
		t.Fatalf("code=%q", wrap.Error.Code)
	}
}

func TestResumeUnknownRun(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})

	resume := httptest.NewRequest(http.MethodPost, "/v0/runs/missing/resume",
		jsonBody(t, map[string]any{"decision": "approve"}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, resume)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rr.Code)
	}
}

func newHITLServer(t *testing.T) (store.Store, *tool.Registry, *run.Engine, http.Handler) {
	t.Helper()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "create_ticket"}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"id": "1"}, false, nil
	}, true)
	eng := &run.Engine{
		Store: st,
		LLM:   &scriptLLM{},
		Tools: reg,
		Gate:  run.NewGate(),
	}
	srv := api.NewServer(st, reg, eng)
	return st, reg, eng, srv.Handler()
}

func pollRunStatus(t *testing.T, h http.Handler, runID string, want store.Status) store.Run {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last store.Run
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+runID, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code == http.StatusOK {
			if err := json.NewDecoder(rr.Body).Decode(&last); err != nil {
				t.Fatal(err)
			}
			if last.Status == want {
				return last
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("status=%q want %s", last.Status, want)
	return last
}

func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(b)
}
