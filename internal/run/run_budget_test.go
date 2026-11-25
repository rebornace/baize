package run

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type usageToolsLLM struct{ calls int }

func (u *usageToolsLLM) Chat(_ context.Context, _ []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	u.calls++
	usage := llm.Usage{TotalTokens: 100}
	if len(tools) == 0 {
		return llm.Message{Role: llm.RoleAssistant, Content: "预算用尽，先给出结论。", Usage: usage}, nil
	}
	return llm.Message{
		Role:      llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("c%d", u.calls), Name: "ping", Arguments: map[string]any{}}},
		Usage:     usage,
	}, nil
}

func (u *usageToolsLLM) SupportsVision() bool { return false }

func TestEngineTokenBudgetUsesGrace(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.Register("ping", func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	ag := agent.Def{ID: "agent", System: "helper"}
	runRec, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "长任务"})
	if err != nil {
		t.Fatal(err)
	}
	llmStub := &usageToolsLLM{}
	eng := &Engine{Store: st, LLM: llmStub, Tools: reg, MaxSteps: 10, MaxTotalTokens: 250}
	if err := eng.Execute(context.Background(), runRec.ID, ag, runRec.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got, err := st.GetRun(runRec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s err=%q", got.Status, got.Error)
	}
	if !strings.Contains(got.Output, "结论") {
		t.Fatalf("output=%q", got.Output)
	}
	// 3 tool turns (300 tokens) then 1 grace turn.
	if llmStub.calls != 4 {
		t.Fatalf("calls=%d want 4", llmStub.calls)
	}
	evs, err := st.ListEvents(runRec.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range evs {
		if ev.Type != EventRunBudgetExceeded {
			continue
		}
		found = true
		if reason := asString(ev.Data["reason"]); reason != "max_total_tokens=250" {
			t.Fatalf("reason=%q", reason)
		}
	}
	if !found {
		t.Fatal("run.budget_exceeded event missing")
	}
}

func TestEffectiveRunBudgetUsesRuntimeKnobs(t *testing.T) {
	eng := &Engine{
		MaxDuration:    time.Minute,
		MaxTotalTokens: 100,
		Settings: fakeKnobs{k: runtimecfg.Knobs{
			MaxDurationSec: 30, MaxTotalTokens: 500,
		}},
	}
	dur, tokens := eng.effectiveRunBudget()
	if dur != 30*time.Second || tokens != 500 {
		t.Fatalf("budget=%s/%d", dur, tokens)
	}
}

func TestRunBudgetReason(t *testing.T) {
	if got := (&Engine{}).runBudgetReason(time.Now().Add(-time.Second), 0, time.Minute, 0); !strings.Contains(got, "max_duration") {
		t.Fatalf("duration reason=%q", got)
	}
	if got := (&Engine{}).runBudgetReason(time.Time{}, 100, 0, 100); !strings.Contains(got, "max_total_tokens") {
		t.Fatalf("token reason=%q", got)
	}
	if got := (&Engine{}).runBudgetReason(time.Time{}, 0, 0, 0); got != "" {
		t.Fatalf("unexpected reason=%q", got)
	}
}

func TestExistingRunTokensCountsResumeUsage(t *testing.T) {
	st := store.NewMemory()
	runRec, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendEvent(runRec.ID, store.Event{
		Type: EventLLMUsage,
		Data: map[string]any{"turn": 0, "total_tokens": 120},
	}); err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, MaxTotalTokens: 100}
	if got := eng.existingRunTokens(runRec.ID, 100); got != 120 {
		t.Fatalf("tokens=%d want 120", got)
	}
}
