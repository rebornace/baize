package run

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// usageScriptLLM returns a tool-calling turn then a terminal text turn, each
// carrying real provider usage.
type usageScriptLLM struct{ calls int }

func (s *usageScriptLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	s.calls++
	if s.calls == 1 {
		return llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: "create_ticket", Arguments: map[string]any{"title": "x"}},
			},
			Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
		}, nil
	}
	return llm.Message{
		Role:    llm.RoleAssistant,
		Content: "已创建",
		Usage:   llm.Usage{PromptTokens: 20, CompletionTokens: 6, TotalTokens: 26},
	}, nil
}

func (s *usageScriptLLM) SupportsVision() bool { return false }

// TestEnginePersistsPerTurnUsage verifies every successful LLM call records
// its real usage: tool-call turns emit a dedicated llm.usage event, while the
// terminal turn embeds usage on llm.message. Aggregating across the run yields
// the total.
func TestEnginePersistsPerTurnUsage(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.Register("create_ticket", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"id": "1"}, false, nil
	})

	ag := agent.Def{ID: "ticket-agent", System: "you are a ticket helper"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "创建工单"})
	if err != nil {
		t.Fatal(err)
	}

	eng := &Engine{Store: st, LLM: &usageScriptLLM{}, Tools: reg}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Tool-call turn usage must be persisted as its own event.
	var usageEvents []store.Event
	var terminal *store.Event
	for i := range evs {
		switch evs[i].Type {
		case EventLLMUsage:
			usageEvents = append(usageEvents, evs[i])
		case EventLLMMessage:
			terminal = &evs[i]
		}
	}

	if len(usageEvents) != 1 {
		t.Fatalf("want 1 llm.usage event, got %d; events=%+v", len(usageEvents), evs)
	}
	if got := asInt(usageEvents[0].Data, "prompt_tokens"); got != 10 {
		t.Fatalf("usage event prompt_tokens=%d want 10", got)
	}
	if got := asInt(usageEvents[0].Data, "completion_tokens"); got != 4 {
		t.Fatalf("usage event completion_tokens=%d want 4", got)
	}
	if got := asInt(usageEvents[0].Data, "total_tokens"); got != 14 {
		t.Fatalf("usage event total_tokens=%d want 14", got)
	}

	if terminal == nil {
		t.Fatal("missing terminal llm.message")
	}
	if got := asInt(terminal.Data, "prompt_tokens"); got != 20 {
		t.Fatalf("terminal prompt_tokens=%d want 20", got)
	}
	if got := asInt(terminal.Data, "completion_tokens"); got != 6 {
		t.Fatalf("terminal completion_tokens=%d want 6", got)
	}
	if got := asInt(terminal.Data, "total_tokens"); got != 26 {
		t.Fatalf("terminal total_tokens=%d want 26", got)
	}
}

func asInt(data map[string]any, key string) int {
	v, ok := data[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
