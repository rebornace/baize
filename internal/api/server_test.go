package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/identity"
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

func (s *scriptLLM) SupportsVision() bool { return false }

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
			"type":             "openapi",
			"spec":             specPath,
			"base_url":         "http://127.0.0.1:18080",
			"require_approval": []string{"create_ticket"},
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
			"type":             "openapi",
			"spec":             specPath,
			"base_url":         "http://127.0.0.1:18080",
			"require_approval": []string{"create_ticket"},
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
	var connBody struct {
		ID              string      `json:"id"`
		Type            string      `json:"type"`
		Spec            string      `json:"spec"`
		BaseURL         string      `json:"base_url"`
		RequireApproval []string    `json:"require_approval"`
		Tools           []tool.Info `json:"tools"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&connBody); err != nil {
		t.Fatal(err)
	}
	if connBody.ID != "ticket" || connBody.Type != "openapi" || connBody.Spec != specPath {
		t.Fatalf("connector body=%+v", connBody)
	}
	if connBody.BaseURL != "http://127.0.0.1:18080" {
		t.Fatalf("base_url=%q", connBody.BaseURL)
	}
	if len(connBody.RequireApproval) != 1 || connBody.RequireApproval[0] != "create_ticket" {
		t.Fatalf("require_approval=%v", connBody.RequireApproval)
	}
	if len(connBody.Tools) != 1 || connBody.Tools[0].Name != "create_ticket" {
		t.Fatalf("connector tools=%+v", connBody.Tools)
	}
	if connBody.Tools[0].Method == "" || connBody.Tools[0].Path == "" {
		t.Fatalf("connector tool missing method/path: %+v", connBody.Tools[0])
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

	listed := reg.List()
	if len(listed) == 0 {
		t.Fatal("ticket-a registry empty after conflict")
	}
	for _, info := range listed {
		if info.ConnectorID != "ticket-a" {
			t.Fatalf("unexpected tool after conflict: %+v", info)
		}
	}
	if _, err := st.GetConnector("ticket-b"); err == nil {
		t.Fatal("store should not contain ticket-b after conflict")
	}
	a, err := st.GetConnector("ticket-a")
	if err != nil || a.BaseURL != "http://a" || a.Spec != specPath {
		t.Fatalf("ticket-a store=%+v err=%v", a, err)
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

	badSpec := filepath.Join(dir, "missing.yaml")
	putBad := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{"type": "openapi", "spec": badSpec, "base_url": "http://evil"}))
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

	c, err := st.GetConnector("ticket")
	if err != nil {
		t.Fatal(err)
	}
	if c.Spec != goodSpec || c.BaseURL != "http://x" {
		t.Fatalf("store overwritten after bad put: %+v", c)
	}
}

func TestPutConnectorEmptyOpsLeavesRegistry(t *testing.T) {
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
	emptySpec := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(emptySpec, []byte(`openapi: 3.0.3
info: { title: empty, version: 0.1.0 }
paths: {}
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

	putEmpty := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{"type": "openapi", "spec": emptySpec, "base_url": "http://empty"}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, putEmpty)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty-ops put status=%d want 400 body=%s", rr.Code, rr.Body.String())
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
		t.Fatalf("tools after empty-ops put=%+v want create_ticket", toolsBody.Tools)
	}

	c, err := st.GetConnector("ticket")
	if err != nil {
		t.Fatal(err)
	}
	if c.Spec != goodSpec || c.BaseURL != "http://x" {
		t.Fatalf("store overwritten after empty-ops put: %+v", c)
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
		ID              string      `json:"id"`
		Type            string      `json:"type"`
		Spec            string      `json:"spec"`
		BaseURL         string      `json:"base_url"`
		RequireApproval []string    `json:"require_approval"`
		Tools           []tool.Info `json:"tools"`
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
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
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

func TestPostRunConversationID(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	st.UpsertAgent(store.Agent{ID: "a", System: "s"})

	postWith := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{
			"agent_id":        "a",
			"input":           "hi",
			"conversation_id": "conv_client",
			"identity_id":     "  idt_force  ",
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, postWith)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var withConv map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&withConv); err != nil {
		t.Fatal(err)
	}
	if withConv["conversation_id"] != "conv_client" {
		t.Fatalf("conversation_id=%v", withConv["conversation_id"])
	}
	runID, _ := withConv["run_id"].(string)
	got, err := st.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConversationID != "conv_client" || got.IdentityID != "idt_force" {
		t.Fatalf("run=%+v want identity_id trimmed to idt_force", got)
	}

	postOmit := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "a", "input": "hi2"}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, postOmit)
	if rr.Code != http.StatusOK {
		t.Fatalf("omit status=%d body=%s", rr.Code, rr.Body.String())
	}
	var omitted map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&omitted); err != nil {
		t.Fatal(err)
	}
	conv, _ := omitted["conversation_id"].(string)
	if conv != "" {
		t.Fatalf("omitted conversation_id must stay empty (machine path), got %q", conv)
	}
	runID2, _ := omitted["run_id"].(string)
	got2, err := st.GetRun(runID2)
	if err != nil {
		t.Fatal(err)
	}
	if got2.ConversationID != "" {
		t.Fatalf("stored conversation_id=%q want empty", got2.ConversationID)
	}
}

func TestConversationIdentitiesAPI(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	idStore := identity.NewMemoryStore()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	srv.Identities = idStore
	h := srv.Handler()

	getEmpty := httptest.NewRequest(http.MethodGet, "/v0/conversations/conv1/identities", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, getEmpty)
	if rr.Code != http.StatusOK {
		t.Fatalf("get empty status=%d body=%s", rr.Code, rr.Body.String())
	}
	var empty []identity.PublicView
	if err := json.NewDecoder(rr.Body).Decode(&empty); err != nil {
		t.Fatal(err)
	}
	if empty == nil {
		t.Fatal("expected JSON array, got null")
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty, got %+v", empty)
	}

	now := time.Now().UTC()
	iid, err := idStore.Upsert("conv1", identity.Identity{
		Label:             "admin@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer SECRET_TOKEN_XYZ"},
		Source:            identity.SourceLoginCapture,
		Subject:           "admin@x.com",
		IsDefault:         false,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || iid == "" {
		t.Fatalf("upsert: id=%q err=%v", iid, err)
	}

	getList := httptest.NewRequest(http.MethodGet, "/v0/conversations/conv1/identities", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getList)
	if rr.Code != http.StatusOK {
		t.Fatalf("get list status=%d body=%s", rr.Code, rr.Body.String())
	}
	raw := rr.Body.String()
	if strings.Contains(raw, "SECRET_TOKEN_XYZ") {
		t.Fatalf("list leaked token: %s", raw)
	}
	var list []identity.PublicView
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != iid || list[0].Label != "admin@x.com" {
		t.Fatalf("list=%+v", list)
	}
	if list[0].IsDefault {
		t.Fatal("expected not default before set")
	}

	setDef := httptest.NewRequest(http.MethodPost, "/v0/conversations/conv1/identities/"+iid+"/default", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, setDef)
	if rr.Code != http.StatusOK {
		t.Fatalf("set default status=%d body=%s", rr.Code, rr.Body.String())
	}
	views := idStore.ListPublic("conv1")
	if len(views) != 1 || !views[0].IsDefault {
		t.Fatalf("after default: %+v", views)
	}

	delOne := httptest.NewRequest(http.MethodDelete, "/v0/conversations/conv1/identities/"+iid, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, delOne)
	if rr.Code != http.StatusOK && rr.Code != http.StatusNoContent {
		t.Fatalf("delete one status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(idStore.ListPublic("conv1")) != 0 {
		t.Fatal("expected empty after delete one")
	}

	_, err = idStore.Upsert("conv1", identity.Identity{
		Label:             "u2@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer OTHER_SECRET"},
		Source:            identity.SourceLoginCapture,
		Subject:           "u2@x.com",
		IsDefault:         true,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = idStore.Upsert("conv1", identity.Identity{
		Label:             "env-default",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer ENV_SECRET"},
		Source:            identity.SourceEnv,
		Subject:           "env",
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		t.Fatal(err)
	}

	clear := httptest.NewRequest(http.MethodDelete, "/v0/conversations/conv1/identities", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, clear)
	if rr.Code != http.StatusOK && rr.Code != http.StatusNoContent {
		t.Fatalf("clear status=%d body=%s", rr.Code, rr.Body.String())
	}
	left := idStore.ListPublic("conv1")
	if len(left) != 1 || left[0].Source != identity.SourceEnv {
		t.Fatalf("clear should keep env: %+v", left)
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

func TestListMessagesNilStore(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/conv1/messages", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var msgs []conversation.Message
	if err := json.NewDecoder(rr.Body).Decode(&msgs); err != nil {
		t.Fatal(err)
	}
	if msgs == nil {
		t.Fatal("expected non-nil JSON array, got null")
	}
	if len(msgs) != 0 {
		t.Fatalf("expected empty, got %+v", msgs)
	}
}

func TestListMessagesEmptyStore(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	srv.Messages = conversation.NewMemoryStore()
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/conv1/messages", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var msgs []conversation.Message
	if err := json.NewDecoder(rr.Body).Decode(&msgs); err != nil {
		t.Fatal(err)
	}
	if msgs == nil || len(msgs) != 0 {
		t.Fatalf("expected empty non-nil array, got %+v", msgs)
	}
}

func TestListMessagesSnakeCase(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	msgStore := conversation.NewMemoryStore()
	_, _ = msgStore.Append("conv1", conversation.Message{
		Role: conversation.RoleUser, Content: "hi", RunID: "run_1",
	})
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	srv.Messages = msgStore
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/conv1/messages", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"conversation_id":"conv1"`) {
		t.Fatalf("missing snake_case conversation_id: %s", body)
	}
	if !strings.Contains(body, `"run_id":"run_1"`) {
		t.Fatalf("missing snake_case run_id: %s", body)
	}
	if !strings.Contains(body, `"created_at"`) {
		t.Fatalf("missing snake_case created_at: %s", body)
	}
	if !strings.Contains(body, `"role":"user"`) {
		t.Fatalf("missing snake_case role: %s", body)
	}
}

func TestClearMessages(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	msgStore := conversation.NewMemoryStore()
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "hi"})
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleAssistant, Content: "hello"})
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	srv.Messages = msgStore
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodDelete, "/v0/conversations/conv1/messages", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(msgStore.List("conv1")) != 0 {
		t.Fatal("expected empty after clear")
	}

	// Clearing again should still succeed (idempotent).
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, httptest.NewRequest(http.MethodDelete, "/v0/conversations/conv1/messages", nil))
	if rr2.Code != http.StatusOK {
		t.Fatalf("second clear status=%d", rr2.Code)
	}
}

func TestClearMessagesNilStore(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodDelete, "/v0/conversations/conv1/messages", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("resp=%+v", resp)
	}
}

func TestPostRunAppendsUserMessage(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	msgStore := conversation.NewMemoryStore()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	srv.Messages = msgStore
	h := srv.Handler()

	st.UpsertAgent(store.Agent{ID: "a", System: "s"})

	postRun := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{
			"agent_id":        "a",
			"input":           "hello",
			"conversation_id": "conv_x",
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, postRun)
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

	msgs := msgStore.List("conv_x")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 user message, got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != conversation.RoleUser || msgs[0].Content != "hello" {
		t.Fatalf("msg=%+v", msgs[0])
	}
	if msgs[0].RunID != runID {
		t.Fatalf("RunID=%q want %q", msgs[0].RunID, runID)
	}
	if msgs[0].ConversationID != "conv_x" {
		t.Fatalf("ConversationID=%q want conv_x", msgs[0].ConversationID)
	}

	// GET /v0/conversations/conv_x/messages reflects the appended user message.
	getMsgs := httptest.NewRequest(http.MethodGet, "/v0/conversations/conv_x/messages", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getMsgs)
	if rr.Code != http.StatusOK {
		t.Fatalf("get messages status=%d", rr.Code)
	}
	var listed []conversation.Message
	if err := json.NewDecoder(rr.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Content != "hello" {
		t.Fatalf("listed=%+v", listed)
	}
}

func TestPostRunNoMessageStoreSkipsAppend(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	// srv.Messages intentionally left nil.
	h := srv.Handler()

	st.UpsertAgent(store.Agent{ID: "a", System: "s"})

	postRun := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "a", "input": "hi", "conversation_id": "conv_y"}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, postRun)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	// GET messages on a nil store returns an empty array, not null.
	getMsgs := httptest.NewRequest(http.MethodGet, "/v0/conversations/conv_y/messages", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getMsgs)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var listed []conversation.Message
	if err := json.NewDecoder(rr.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if listed == nil || len(listed) != 0 {
		t.Fatalf("expected empty non-nil array, got %+v", listed)
	}
}

// failLeaveRunningRunner returns an error without updating run status, so the
// API Execute兜底 path marks failed and writes system_note.
type failLeaveRunningRunner struct{}

func (f *failLeaveRunningRunner) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	return fmt.Errorf("boom before status update")
}

func (f *failLeaveRunningRunner) ContinueFromHITL(ctx context.Context, runID string, d run.Decision) error {
	return nil
}

func TestPostRunAPIFallbackWritesSystemNote(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	msgStore := conversation.NewMemoryStore()
	srv := api.NewServer(st, reg, &failLeaveRunningRunner{})
	srv.Messages = msgStore
	h := srv.Handler()

	st.UpsertAgent(store.Agent{ID: "a", System: "s"})

	postRun := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{
			"agent_id":        "a",
			"input":           "hello",
			"conversation_id": "conv_fail",
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, postRun)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	var note *conversation.Message
	for time.Now().Before(deadline) {
		msgs := msgStore.List("conv_fail")
		for i := range msgs {
			if msgs[i].Role == conversation.RoleSystemNote {
				note = &msgs[i]
				break
			}
		}
		if note != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if note == nil {
		t.Fatalf("expected system_note after API fallback; msgs=%+v", msgStore.List("conv_fail"))
	}
	if !strings.HasPrefix(note.Content, "运行失败：") || !strings.Contains(note.Content, "boom before status update") {
		t.Fatalf("note.Content=%q", note.Content)
	}
}

func TestListConversations(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	msgs := conversation.NewMemoryStore()
	srv.Messages = msgs
	_, _ = msgs.Append("conv_a", conversation.Message{Role: conversation.RoleUser, Content: "VPN 挂了"})

	req := httptest.NewRequest(http.MethodGet, "/v0/conversations", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Conversations []conversation.Summary `json:"conversations"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Conversations) != 1 || body.Conversations[0].ID != "conv_a" || body.Conversations[0].Title != "VPN 挂了" {
		t.Fatalf("%+v", body)
	}
}

func TestListConversationsNilStore(t *testing.T) {
	st := store.NewMemory()
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	req := httptest.NewRequest(http.MethodGet, "/v0/conversations", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"conversations"`) {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestRunStreamReplayAndAfter(t *testing.T) {
	mem := store.NewMemory()
	hub := eventbus.NewHub()
	st := eventbus.Notify(mem, hub)
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.Hub = hub
	run, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	_ = st.AppendEvent(run.ID, store.Event{Type: "run.started"})
	_ = st.AppendEvent(run.ID, store.Event{Type: "llm.message", Data: map[string]any{"content": "hi"}})

	req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+run.ID+"/stream?after=0", nil)
	rr := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	done := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(rr, req)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	_ = st.UpdateRun(run.ID, store.StatusSucceeded, "x", "")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("stream did not end")
	}
	cancel()
	body := rr.Body.String()
	if rr.Code != 200 {
		t.Fatalf("code=%d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("ct=%s", rr.Header().Get("Content-Type"))
	}
	if !strings.Contains(body, "llm.message") || !strings.Contains(body, "run.ended") {
		t.Fatalf("body=%s", body)
	}
	if strings.Contains(body, "run.started") {
		t.Fatalf("after=0 should skip index 0: %s", body)
	}
}

func TestRunStreamNotFound(t *testing.T) {
	st := store.NewMemory()
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	req := httptest.NewRequest(http.MethodGet, "/v0/runs/nope/stream", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "run_not_found") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

// TestRunStreamLastEventWithImmediateEnd ensures AppendEvent then UpdateRun
// back-to-back still delivers the final event before run.ended (select race).
func TestRunStreamLastEventWithImmediateEnd(t *testing.T) {
	mem := store.NewMemory()
	hub := eventbus.NewHub()
	st := eventbus.Notify(mem, hub)
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.Hub = hub
	run, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(run.ID, store.Event{Type: "run.started"})

	req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+run.ID+"/stream?after=-1", nil)
	rr := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)
	done := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(rr, req)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	_ = st.AppendEvent(run.ID, store.Event{Type: "llm.message", Data: map[string]any{"content": "bye"}})
	_ = st.UpdateRun(run.ID, store.StatusSucceeded, "ok", "")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("stream did not end")
	}
	body := rr.Body.String()
	if !strings.Contains(body, "llm.message") {
		t.Fatalf("missing last event before end: %s", body)
	}
	if !strings.Contains(body, "run.ended") {
		t.Fatalf("missing run.ended: %s", body)
	}
	if idxMsg, idxEnd := strings.Index(body, "llm.message"), strings.Index(body, "run.ended"); idxMsg < 0 || idxEnd < 0 || idxMsg > idxEnd {
		t.Fatalf("llm.message should appear before run.ended: %s", body)
	}
}

// windowListStore blocks the first ListEvents after taking a snapshot so the
// test can AppendEvent+UpdateRun in the ListEvents→Subscribe gap (fan-out dropped).
type windowListStore struct {
	store.Store
	listCalls    atomic.Int32
	afterFirst   chan struct{}
	releaseFirst chan struct{}
}

func (w *windowListStore) ListEvents(runID string) ([]store.Event, error) {
	evs, err := w.Store.ListEvents(runID)
	if w.listCalls.Add(1) == 1 {
		close(w.afterFirst)
		<-w.releaseFirst
	}
	return evs, err
}

// TestRunStreamCatchUpAfterSubscribeWindow proves that events + terminal status
// published between first ListEvents and Subscribe still close the stream.
func TestRunStreamCatchUpAfterSubscribeWindow(t *testing.T) {
	mem := store.NewMemory()
	gate := &windowListStore{
		Store:        mem,
		afterFirst:   make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	hub := eventbus.NewHub()
	st := eventbus.Notify(gate, hub)
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.Hub = hub
	run, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+run.ID+"/stream", nil)
	rr := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)
	done := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(rr, req)
		close(done)
	}()

	select {
	case <-gate.afterFirst:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("ListEvents did not start")
	}
	_ = st.AppendEvent(run.ID, store.Event{Type: "llm.message", Data: map[string]any{"content": "late"}})
	_ = st.UpdateRun(run.ID, store.StatusSucceeded, "ok", "")
	close(gate.releaseFirst)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("stream did not end after catch-up")
	}
	body := rr.Body.String()
	if rr.Code != 200 {
		t.Fatalf("code=%d body=%s", rr.Code, body)
	}
	if !strings.Contains(body, "llm.message") || !strings.Contains(body, "run.ended") {
		t.Fatalf("expected late event and run.ended: %s", body)
	}
}

const putCaptureJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhZG1pbkB4LmNvbSIsImVtYWlsIjoiYWRtaW5AeC5jb20iLCJyb2xlcyI6WyJhZG1pbiJdLCJleHAiOjk5OTk5OTk5OTl9.sig"

func writeLoginGetMeAPISpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "spec.yaml")
	content := `openapi: 3.0.3
info:
  title: login-getme
  version: 0.1.0
components:
  securitySchemes:
    bearer:
      type: http
      scheme: bearer
paths:
  /login:
    post:
      operationId: login
      security: []
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                email: { type: string }
                password: { type: string }
      responses:
        "200":
          description: ok
  /me:
    get:
      operationId: getMe
      security:
        - bearer: []
      responses:
        "200":
          description: ok
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPutConnectorWiresCaptureWithoutRestart(t *testing.T) {
	var lastAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_, _ = w.Write([]byte(`{"accessToken":"` + putCaptureJWT + `","email":"admin@x.com"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/me":
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)

	spec := writeLoginGetMeAPISpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	srv.Identities = identity.NewMemoryStore()
	h := srv.Handler()

	put := httptest.NewRequest(http.MethodPut, "/v0/connectors/auth",
		jsonBody(t, map[string]any{
			"type":     "openapi",
			"spec":     spec,
			"base_url": upstream.URL,
			"auth": map[string]any{
				"mode": "static",
				"static": map[string]any{
					"headers": map[string]string{"Authorization": "Bearer PUT_STATIC"},
				},
			},
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", rr.Code, rr.Body.String())
	}

	ctx := identity.WithConversationID(context.Background(), "conv_put_cap")
	_, isErr, err := reg.Invoke(ctx, "login", map[string]any{"email": "admin@x.com", "password": "x"})
	if err != nil || isErr {
		t.Fatalf("login: isErr=%v err=%v", isErr, err)
	}
	lastAuth = ""
	_, isErr, err = reg.Invoke(ctx, "getMe", nil)
	if err != nil || isErr {
		t.Fatalf("getMe: isErr=%v err=%v", isErr, err)
	}
	want := "Bearer " + putCaptureJWT
	if lastAuth != want {
		t.Fatalf("Authorization=%q want %q (not PUT_STATIC)", lastAuth, want)
	}
	if lastAuth == "Bearer PUT_STATIC" {
		t.Fatal("must use captured JWT, not PUT static")
	}
}

func TestPutConnectorNoneCaptureDisables(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/login" {
			_, _ = w.Write([]byte(`{"accessToken":"` + putCaptureJWT + `","email":"admin@x.com"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)

	spec := writeLoginGetMeAPISpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	ids := identity.NewMemoryStore()
	srv.Identities = ids
	h := srv.Handler()

	put := httptest.NewRequest(http.MethodPut, "/v0/connectors/auth",
		jsonBody(t, map[string]any{
			"type":     "openapi",
			"spec":     spec,
			"base_url": upstream.URL,
			"auth": map[string]any{
				"mode": "static",
				"capture": map[string]any{
					"tool_name_glob": "__none__",
				},
			},
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", rr.Code, rr.Body.String())
	}

	ctx := identity.WithConversationID(context.Background(), "conv_none")
	_, isErr, err := reg.Invoke(ctx, "login", map[string]any{"email": "a@x.com", "password": "x"})
	if err != nil || isErr {
		t.Fatalf("login: isErr=%v err=%v", isErr, err)
	}
	if len(ids.List("conv_none")) != 0 {
		t.Fatalf("identities should stay empty with __none__, got %+v", ids.List("conv_none"))
	}
}

func TestPutOmitsRequireLoginPreserves(t *testing.T) {
	spec := writeTicketSpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	put1 := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":          "openapi",
			"spec":          spec,
			"base_url":      "http://x",
			"require_login": []string{"create_ticket"},
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put1)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT1 status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !reg.RequiresLogin("create_ticket") {
		t.Fatal("create_ticket should require login after first PUT")
	}

	put2 := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":     "openapi",
			"spec":     spec,
			"base_url": "http://x",
		}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put2)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT2 status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !reg.RequiresLogin("create_ticket") {
		t.Fatal("omitting require_login must preserve create_ticket")
	}
}

func TestPutEmptyRequireLoginClears(t *testing.T) {
	spec := writeTicketSpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	put1 := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":          "openapi",
			"spec":          spec,
			"base_url":      "http://x",
			"require_login": []string{"create_ticket"},
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put1)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT1 status=%d body=%s", rr.Code, rr.Body.String())
	}

	put2 := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":          "openapi",
			"spec":          spec,
			"base_url":      "http://x",
			"require_login": []string{},
		}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put2)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT2 status=%d body=%s", rr.Code, rr.Body.String())
	}
	if reg.RequiresLogin("create_ticket") {
		t.Fatal(`"require_login":[] must clear create_ticket`)
	}
}

func TestPatchToolRequireLogin(t *testing.T) {
	spec := writeTicketSpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	put := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":     "openapi",
			"spec":     spec,
			"base_url": "http://x",
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", rr.Code, rr.Body.String())
	}

	patchOn := httptest.NewRequest(http.MethodPatch, "/v0/tools/create_ticket",
		jsonBody(t, map[string]any{"require_login": true}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, patchOn)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH on status=%d body=%s", rr.Code, rr.Body.String())
	}

	getTools := httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getTools)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET tools status=%d", rr.Code)
	}
	var toolsBody struct {
		Tools []tool.Info `json:"tools"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&toolsBody); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, info := range toolsBody.Tools {
		if info.Name == "create_ticket" {
			found = true
			if !info.RequireLogin {
				t.Fatal("GET /v0/tools: create_ticket require_login want true")
			}
		}
	}
	if !found {
		t.Fatal("create_ticket missing from GET /v0/tools")
	}

	patchOff := httptest.NewRequest(http.MethodPatch, "/v0/tools/create_ticket",
		jsonBody(t, map[string]any{"require_login": false}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, patchOff)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH off status=%d body=%s", rr.Code, rr.Body.String())
	}
	if reg.RequiresLogin("create_ticket") {
		t.Fatal("after PATCH false, RequiresLogin should be false")
	}

	patchMissing := httptest.NewRequest(http.MethodPatch, "/v0/tools/no_such_tool",
		jsonBody(t, map[string]any{"require_login": true}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, patchMissing)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown tool status=%d want 404", rr.Code)
	}
	var wrap404 struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap404); err != nil {
		t.Fatal(err)
	}
	if wrap404.Error.Code != "not_found" {
		t.Fatalf("code=%q want not_found", wrap404.Error.Code)
	}

	patchEmpty := httptest.NewRequest(http.MethodPatch, "/v0/tools/create_ticket",
		jsonBody(t, map[string]any{}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, patchEmpty)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("{} status=%d want 400", rr.Code)
	}
	var wrap400 struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap400); err != nil {
		t.Fatal(err)
	}
	if wrap400.Error.Code != "invalid_request" {
		t.Fatalf("code=%q want invalid_request", wrap400.Error.Code)
	}
}

func TestPutHTTPPluginPreservesCaptureAndUsesIdentity(t *testing.T) {
	var lastAuth string
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"echo"}]}`))
		case strings.HasSuffix(r.URL.Path, "/invoke"):
			lastAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(sidecar.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	ids := identity.NewMemoryStore()
	srv.Identities = ids
	if _, err := ids.Upsert("conv_http_put", identity.Identity{
		Scheme:            "bearer",
		Subject:           "u",
		CredentialHeaders: map[string]string{"Authorization": "Bearer FROM_ID"},
		Source:            identity.SourceLoginCapture,
		IsDefault:         true,
	}); err != nil {
		t.Fatal(err)
	}

	put := httptest.NewRequest(http.MethodPut, "/v0/connectors/side",
		jsonBody(t, map[string]any{
			"type":     "http",
			"base_url": sidecar.URL,
			"auth": map[string]any{
				"mode": "static",
				"static": map[string]any{
					"headers": map[string]string{"Authorization": "Bearer PUT_STATIC"},
				},
				"capture": map[string]any{
					"tool_name_glob": "*login*",
				},
			},
		}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", rr.Code, rr.Body.String())
	}

	c, err := st.GetConnector("side")
	if err != nil {
		t.Fatal(err)
	}
	if c.Auth.Capture.ToolNameGlob != "*login*" {
		t.Fatalf("HTTP PUT must persist Capture, got %+v", c.Auth.Capture)
	}

	ctx := identity.WithConversationID(context.Background(), "conv_http_put")
	lastAuth = ""
	_, isErr, invErr := reg.Invoke(ctx, "echo", nil)
	if invErr != nil || isErr {
		t.Fatalf("invoke: isErr=%v err=%v", isErr, invErr)
	}
	if lastAuth != "Bearer FROM_ID" {
		t.Fatalf("Authorization=%q want Bearer FROM_ID", lastAuth)
	}
	if lastAuth == "Bearer PUT_STATIC" {
		t.Fatal("must not use static default in conversation")
	}
}
