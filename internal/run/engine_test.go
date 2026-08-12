package run

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

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

func TestEngineReActToolThenMessage(t *testing.T) {
	st := store.New()
	reg := tool.NewRegistry()
	reg.Register("create_ticket", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"id": "1"}, false, nil
	})

	ag := agent.Def{ID: "ticket-agent", System: "you are a ticket helper"}
	r, err := st.CreateRun(ag.ID, "创建工单")
	if err != nil {
		t.Fatal(err)
	}

	eng := &Engine{Store: st, LLM: &scriptLLM{}, Tools: reg}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got, err := st.GetRun(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s want succeeded", got.Status)
	}
	if got.Output != "已创建" {
		t.Fatalf("output=%q", got.Output)
	}

	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	types := make(map[string]bool)
	for _, ev := range evs {
		types[ev.Type] = true
	}
	for _, want := range []string{"run.started", "llm.tool_call", "tool.result", "llm.message"} {
		if !types[want] {
			t.Fatalf("missing event %s; events=%+v", want, evs)
		}
	}
}
