package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type fakeRunner struct {
	store *store.Store
}

func (f *fakeRunner) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	_ = f.store.AppendEvent(runID, store.Event{Type: "run.started"})
	return f.store.UpdateRun(runID, store.StatusSucceeded, "已创建", "")
}

func TestHealthz(t *testing.T) {
	st := store.New()
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
	st := store.New()
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
	if created["status"] != string(store.StatusSucceeded) {
		t.Fatalf("status=%v want succeeded", created["status"])
	}

	getRun := httptest.NewRequest(http.MethodGet, "/v0/runs/"+runID, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getRun)
	if rr.Code != http.StatusOK {
		t.Fatalf("get run status=%d", rr.Code)
	}
	var run store.Run
	if err := json.NewDecoder(rr.Body).Decode(&run); err != nil {
		t.Fatal(err)
	}
	if run.Status != store.StatusSucceeded || run.Output != "已创建" {
		t.Fatalf("run=%+v", run)
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
	st := store.New()
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

	st := store.New()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})

	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":     "openapi",
			"spec":     specPath,
			"base_url": "http://127.0.0.1:18080",
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
	c, err := st.GetConnector("ticket")
	if err != nil || c.BaseURL != "http://127.0.0.1:18080" {
		t.Fatalf("connector=%+v err=%v", c, err)
	}
}

func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(b)
}
