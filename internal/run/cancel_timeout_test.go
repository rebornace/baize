package run_test

import (
	"context"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type slowToolLLM struct{}

func (slowToolLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	for _, m := range messages {
		if m.Role == llm.RoleTool {
			return llm.Message{Role: llm.RoleAssistant, Content: "done"}, nil
		}
	}
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
		{ID: "c1", Name: "slow", Arguments: map[string]any{}},
	}}, nil
}

func TestToolInvokeTimeout(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	reg := tool.NewRegistry()
	reg.Register("slow", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		select {
		case <-ctx.Done():
			return nil, true, ctx.Err()
		case <-time.After(2 * time.Second):
			return map[string]any{"ok": true}, false, nil
		}
	})
	eng := &run.Engine{
		Store:       st,
		LLM:         slowToolLLM{},
		Tools:       reg,
		MaxSteps:    4,
		ToolTimeout: 50 * time.Millisecond,
	}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "go"})
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "go"); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s want succeeded (tool error recovered)", got.Status)
	}
	evs, _ := st.ListEvents(r.ID)
	var sawTimeout bool
	for _, ev := range evs {
		if ev.Type != run.EventToolResult {
			continue
		}
		content, _ := ev.Data["content"].(map[string]any)
		errStr, _ := content["error"].(string)
		if ev.Data["is_error"] == true && errStr != "" {
			sawTimeout = true
		}
	}
	if !sawTimeout {
		t.Fatalf("expected tool timeout result; events=%+v", evs)
	}
}

func TestCancelActiveRun(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	reg := tool.NewRegistry()
	started := make(chan struct{})
	reg.Register("block", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		close(started)
		<-ctx.Done()
		return nil, true, ctx.Err()
	})
	llmStub := &blockThenCancelLLM{}
	eng := &run.Engine{
		Store:       st,
		LLM:         llmStub,
		Tools:       reg,
		MaxSteps:    4,
		ToolTimeout: 30 * time.Second,
	}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "go", ConversationID: "c1"})
	done := make(chan error, 1)
	go func() {
		done <- eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "go")
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("tool did not start")
	}
	if err := eng.Cancel(r.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Execute did not return after Cancel")
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusCancelled {
		t.Fatalf("status=%s want cancelled", got.Status)
	}
}

type blockThenCancelLLM struct{}

func (blockThenCancelLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
		{ID: "c1", Name: "block", Arguments: map[string]any{}},
	}}, nil
}
