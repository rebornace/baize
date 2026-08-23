package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestEnterpriseCallbackOpenAPIRun(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "spec.yaml")
	spec := `openapi: 3.0.3
info:
  title: echo
  version: 0.1.0
paths:
  /echo:
    post:
      operationId: echo
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                msg: { type: string }
      responses:
        "200":
          description: ok
`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	var (
		mu   sync.Mutex
		got  map[string]any
		seen bool
	)
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		seen = true
		got = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&got)
		if strings.TrimSpace(r.Header.Get("X-Baize-Protocol")) != "v0" {
			t.Errorf("protocol header missing")
		}
		_, _ = w.Write([]byte(`{"content":{"enterprise":true},"is_error":false}`))
	}))
	t.Cleanup(callback.Close)

	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "agent-ec", System: "helper"})
	reg := tool.NewRegistry()
	engine := &run.Engine{
		Store:    st,
		LLM:      &enterpriseCallbackLLM{toolName: "echo", args: map[string]any{"msg": "hi"}},
		Tools:    reg,
		Gate:     run.NewGate(),
		MaxSteps: 4,
	}
	srv := api.NewServer(st, reg, engine)
	h := srv.Handler()

	putBody, _ := json.Marshal(map[string]any{
		"type":                   "openapi",
		"spec":                   specPath,
		"base_url":               "http://127.0.0.1:1",
		"execution_callback_url": callback.URL + "/execute",
	})
	putReq := httptest.NewRequest(http.MethodPut, "/v0/connectors/legacy", bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, putReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT connector status=%d body=%s", rr.Code, rr.Body.String())
	}

	runBody, _ := json.Marshal(map[string]any{
		"agent_id": "agent-ec",
		"input":    "go",
	})
	postRun := httptest.NewRequest(http.MethodPost, "/v0/runs", bytes.NewReader(runBody))
	postRun.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, postRun)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST run status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)
	if runID == "" {
		t.Fatal("missing run_id")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runRec, err := st.GetRun(runID)
		if err == nil && runRec != nil && runRec.Status == store.StatusSucceeded {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if !seen {
		t.Fatal("enterprise callback was not invoked")
	}
	if got["tool"] != "echo" {
		t.Fatalf("callback body=%v", got)
	}
	if _, ok := got["idempotency_key"].(string); !ok || got["idempotency_key"] == "" {
		t.Fatalf("expected idempotency_key in callback body=%v", got)
	}
}

type enterpriseCallbackLLM struct {
	toolName string
	args     map[string]any
}

func (m *enterpriseCallbackLLM) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSpec) (llm.Message, error) {
	return llm.Message{
		Role: llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{
			ID:        "call_ec_1",
			Name:      m.toolName,
			Arguments: m.args,
		}},
	}, nil
}
