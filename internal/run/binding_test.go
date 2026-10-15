package run

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type bindingReplyLLM struct{}

func (bindingReplyLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	return llm.Message{Role: llm.RoleAssistant, Content: "ok"}, nil
}

func (bindingReplyLLM) SupportsVision() bool { return false }

func TestRunBindingPersistedOnTurnZero(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "c1", Type: "openapi", BaseURL: "http://x", CatalogRevision: 2})
	st.UpsertTool(store.Tool{
		Name: "echo", ConnectorID: "c1", Source: store.ToolSourceSpec, Enabled: true,
		Method: "GET", Path: "/echo", InputSchema: map[string]any{"type": "object"},
	})
	reg := tool.NewRegistry()
	reg.RegisterSpec(llm.ToolSpec{Name: "echo", Description: "e", InputSchema: map[string]any{"type": "object"}},
		func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
			return map[string]any{"ok": true}, false, nil
		})

	ag := agent.Def{ID: "a1", System: "sys"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, LLM: bindingReplyLLM{}, Tools: reg, Gate: NewGate(), MaxSteps: 4}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	evs, _ := st.ListEvents(r.ID)
	var binding map[string]any
	for _, ev := range evs {
		if ev.Type == EventRunBinding {
			binding = ev.Data
			break
		}
	}
	if binding == nil {
		t.Fatal("missing run.binding")
	}
	toolsSent, ok := binding["tools_sent"].([]map[string]any)
	if !ok {
		t.Fatalf("tools_sent type %T", binding["tools_sent"])
	}
	found := false
	for _, row := range toolsSent {
		if row["name"] == "echo" {
			if row["contract_hash"] == "" {
				t.Fatal("empty contract_hash")
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("echo missing in tools_sent: %#v", toolsSent)
	}
	cat, _ := binding["catalog"].(map[string]int64)
	if cat == nil {
		// map[string]any from JSON-like
		if m, ok := binding["catalog"].(map[string]any); ok {
			if v, ok := m["c1"].(int64); ok && v == 2 {
				return
			}
			if v, ok := m["c1"].(int); ok && v == 2 {
				return
			}
			t.Fatalf("catalog=%#v", m)
		}
		t.Fatalf("catalog missing: %#v", binding["catalog"])
	}
	if cat["c1"] != 2 {
		t.Fatalf("catalog revision=%d", cat["c1"])
	}
}
