package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	httppluginex "github.com/rebornace/baize/examples/http-plugin"
	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// TestHTTPPluginPutToolsRun registers the reference HTTP sidecar via PUT,
// checks GET /v0/tools, then runs Engine with a mock LLM that invokes echo,
// asserting the sidecar received X-Baize-Protocol: v0.
func TestHTTPPluginPutToolsRun(t *testing.T) {
	var (
		mu         sync.Mutex
		lastProto  string
		invokeSeen bool
	)
	inner := httppluginex.NewHandler()
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/invoke") {
			mu.Lock()
			lastProto = r.Header.Get("X-Baize-Protocol")
			invokeSeen = true
			mu.Unlock()
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(sidecar.Close)

	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "agent-http", System: "helper"})
	reg := tool.NewRegistry()
	engine := &run.Engine{
		Store:    st,
		LLM:      &httpPluginScriptLLM{toolName: "echo", args: map[string]any{"msg": "hi"}},
		Tools:    reg,
		Gate:     run.NewGate(),
		MaxSteps: 4,
	}
	srv := api.NewServer(st, reg, engine)
	h := srv.Handler()

	putBody, _ := json.Marshal(map[string]any{
		"type":     "http",
		"base_url": sidecar.URL,
	})
	putReq := httptest.NewRequest(http.MethodPut, "/v0/connectors/side", bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, putReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT connector status=%d body=%s", rr.Code, rr.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /v0/tools status=%d body=%s", rr.Code, rr.Body.String())
	}
	var toolsBody struct {
		Tools []tool.Info `json:"tools"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&toolsBody); err != nil {
		t.Fatal(err)
	}
	var foundEcho bool
	for _, info := range toolsBody.Tools {
		if info.Name == "echo" && info.ConnectorID == "side" {
			foundEcho = true
			break
		}
	}
	if !foundEcho {
		t.Fatalf("expected echo with connector_id=side; tools=%+v", toolsBody.Tools)
	}

	ag := agent.Def{ID: "agent-http", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "echo please"})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got, err := st.GetRun(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s want succeeded", got.Status)
	}

	mu.Lock()
	seen, proto := invokeSeen, lastProto
	mu.Unlock()
	if !seen {
		t.Fatal("expected sidecar invoke")
	}
	if proto != "v0" {
		t.Fatalf("X-Baize-Protocol=%q want v0", proto)
	}
}

// TestHTTPPluginHITLRequireApproval verifies require_approval on create_ticket
// leaves the run in waiting_human without invoking the sidecar tool.
func TestHTTPPluginHITLRequireApproval(t *testing.T) {
	var (
		mu         sync.Mutex
		invokeSeen bool
	)
	inner := httppluginex.NewHandler()
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/invoke") {
			mu.Lock()
			invokeSeen = true
			mu.Unlock()
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(sidecar.Close)

	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "agent-hitl", System: "helper"})
	reg := tool.NewRegistry()
	gate := run.NewGate()
	engine := &run.Engine{
		Store:    st,
		LLM:      &httpPluginScriptLLM{toolName: "create_ticket", args: map[string]any{"title": "need approval"}},
		Tools:    reg,
		Gate:     gate,
		MaxSteps: 4,
	}
	srv := api.NewServer(st, reg, engine)
	h := srv.Handler()

	putBody, _ := json.Marshal(map[string]any{
		"type":             "http",
		"base_url":         sidecar.URL,
		"require_approval": []string{"create_ticket"},
	})
	putReq := httptest.NewRequest(http.MethodPut, "/v0/connectors/side", bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, putReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT connector status=%d body=%s", rr.Code, rr.Body.String())
	}

	ag := agent.Def{ID: "agent-hitl", System: "helper"}
	runRec, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "create ticket"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- engine.Execute(ctx, runRec.ID, ag, runRec.Input)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, gerr := st.GetRun(runRec.ID)
		if gerr == nil && got.Status == store.StatusWaitingHuman {
			mu.Lock()
			seen := invokeSeen
			mu.Unlock()
			if seen {
				t.Fatal("create_ticket must not invoke before approval")
			}
			if got.Status != "waiting_human" {
				t.Fatalf("status string=%q", got.Status)
			}
			cancel()
			select {
			case <-errCh:
			case <-time.After(2 * time.Second):
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout waiting for waiting_human")
}

// httpPluginScriptLLM calls toolName once, then returns a final text message.
type httpPluginScriptLLM struct {
	toolName string
	args     map[string]any
	calls    int
}

func (s *httpPluginScriptLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	s.calls++
	if s.calls == 1 {
		args := s.args
		if args == nil {
			args = map[string]any{}
		}
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "call_1", Name: s.toolName, Arguments: args},
		}}, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: "done"}, nil
}
