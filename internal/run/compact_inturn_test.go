package run

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestCompactLiveMessagesFoldsMiddleKeepsRecent(t *testing.T) {
	sumLLM := &fakeCompactLLM{reply: "轮内摘要检查点"}
	ms := conversation.NewMemoryStore()
	c := &Compactor{
		Messages: ms, LLM: sumLLM,
		Profiles:  fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 4000}},
		Threshold: 0.5, ReserveTokens: 200, KeepRecent: 4,
	}
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "sys"},
		{Role: llm.RoleSystem, Content: "memory block"},
		{Role: llm.RoleUser, Content: "用户问题"},
	}
	filler := strings.Repeat("工具大结果内容", 80)
	for i := 0; i < 12; i++ {
		msgs = append(msgs,
			llm.Message{Role: llm.RoleAssistant, Content: "", ToolCalls: []llm.ToolCall{{ID: "c", Name: "read"}}},
			llm.Message{Role: llm.RoleTool, ToolCallID: "c", Content: filler + " OLD-" + string(rune('A'+i%26))},
		)
	}
	// Distinct recent markers in the keep window.
	msgs = append(msgs,
		llm.Message{Role: llm.RoleAssistant, Content: "RECENT-ASSIST"},
		llm.Message{Role: llm.RoleTool, ToolCallID: "r", Content: "RECENT-TOOL-KEEP"},
		llm.Message{Role: llm.RoleAssistant, Content: "RECENT-2"},
		llm.Message{Role: llm.RoleUser, Content: "RECENT-USER"},
	)
	before := EstimateMessagesTokens(msgs)
	changed, err := c.CompactLiveMessages(context.Background(), &msgs, nil, "p", "conv")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected live compaction to rewrite messages")
	}
	after := EstimateMessagesTokens(msgs)
	if after >= before {
		t.Fatalf("expected token drop: before=%d after=%d", before, after)
	}
	foundSummary := false
	for _, m := range msgs {
		if m.Role == llm.RoleSystem && strings.Contains(m.Content, "轮内摘要检查点") {
			foundSummary = true
		}
		if strings.Contains(m.Content, "OLD-A") {
			t.Fatalf("folded old tool text must not remain verbatim: %+v", m)
		}
	}
	if !foundSummary {
		t.Fatalf("in-turn summary system message missing: %+v", msgs)
	}
	if !strings.Contains(msgs[len(msgs)-1].Content, "RECENT-USER") {
		t.Fatalf("keep window must retain recent messages: last=%+v", msgs[len(msgs)-1])
	}
	keptRecent := false
	for _, m := range msgs {
		if strings.Contains(m.Content, "RECENT-TOOL-KEEP") {
			keptRecent = true
		}
	}
	if !keptRecent {
		t.Fatalf("recent tool result must stay verbatim: %+v", msgs)
	}
}

func TestCompactLiveMessagesUnderBudgetNoop(t *testing.T) {
	sumLLM := &fakeCompactLLM{reply: "不应调用"}
	c := &Compactor{
		LLM:       sumLLM,
		Profiles:  fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 128000}},
		Threshold: 0.9, ReserveTokens: 1000, KeepRecent: 8,
	}
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "sys"},
		{Role: llm.RoleUser, Content: "hi"},
	}
	changed, err := c.CompactLiveMessages(context.Background(), &msgs, nil, "p", "")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("under budget must not compact")
	}
	if sumLLM.got != nil {
		t.Fatal("summarizer must not be called under budget")
	}
}

func TestCompactLiveMessagesFailurePropagates(t *testing.T) {
	sumLLM := &fakeCompactLLM{err: errCompactBoom}
	c := &Compactor{
		LLM:       sumLLM,
		Profiles:  fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 4000}},
		Threshold: 0.5, ReserveTokens: 200, KeepRecent: 2,
	}
	msgs := []llm.Message{{Role: llm.RoleSystem, Content: "sys"}}
	filler := strings.Repeat("大", 200)
	for i := 0; i < 10; i++ {
		msgs = append(msgs, llm.Message{Role: llm.RoleTool, Content: filler})
	}
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: "keep1"}, llm.Message{Role: llm.RoleUser, Content: "keep2"})
	_, err := c.CompactLiveMessages(context.Background(), &msgs, nil, "p", "")
	if err == nil {
		t.Fatal("expected summarize error")
	}
}

func TestMaybeCompactInTurnEmitsEvent(t *testing.T) {
	sumLLM := &fakeCompactLLM{reply: "轮内摘要"}
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	ms := conversation.NewMemoryStore()
	eng := &Engine{
		Store: st, Tools: tool.NewRegistry(), MaxSteps: 2, Messages: ms,
		Compactor: &Compactor{
			Messages: ms, LLM: sumLLM,
			Profiles:  fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 4000}},
			Threshold: 0.5, ReserveTokens: 200, KeepRecent: 3,
		},
	}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "q", ConversationID: "c1", ModelProfileID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	msgs := []llm.Message{{Role: llm.RoleSystem, Content: "sys"}, {Role: llm.RoleUser, Content: "q"}}
	filler := strings.Repeat("工具结果膨胀", 100)
	for i := 0; i < 10; i++ {
		msgs = append(msgs, llm.Message{Role: llm.RoleTool, Content: filler})
	}
	msgs = append(msgs,
		llm.Message{Role: llm.RoleAssistant, Content: "a"},
		llm.Message{Role: llm.RoleTool, Content: "KEEP-ME"},
		llm.Message{Role: llm.RoleUser, Content: "u"},
	)
	eng.maybeCompactInTurn(context.Background(), r.ID, &msgs, nil)
	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range evs {
		if ev.Type != EventContextCompacted {
			continue
		}
		found = true
		if phase, _ := ev.Data["phase"].(string); phase != "in_turn" {
			t.Fatalf("phase want in_turn, got %+v", ev.Data)
		}
	}
	if !found {
		t.Fatalf("expected context.compacted with phase=in_turn, events=%v", eventTypes(evs))
	}
}

func TestMaybeCompactInTurnFailureIsBestEffort(t *testing.T) {
	sumLLM := &fakeCompactLLM{err: errCompactBoom}
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a"})
	eng := &Engine{
		Store: st,
		Compactor: &Compactor{
			LLM:       sumLLM,
			Profiles:  fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 4000}},
			Threshold: 0.5, ReserveTokens: 200, KeepRecent: 2,
		},
	}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "q", ModelProfileID: "p"})
	if err != nil {
		t.Fatal(err)
	}
	msgs := []llm.Message{{Role: llm.RoleSystem, Content: "sys"}}
	filler := strings.Repeat("大", 200)
	for i := 0; i < 10; i++ {
		msgs = append(msgs, llm.Message{Role: llm.RoleTool, Content: filler})
	}
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: "k1"}, llm.Message{Role: llm.RoleUser, Content: "k2"})
	origLen := len(msgs)
	eng.maybeCompactInTurn(context.Background(), r.ID, &msgs, nil)
	if len(msgs) != origLen {
		t.Fatalf("on failure messages must be unchanged: got %d want %d", len(msgs), origLen)
	}
	evs, _ := st.ListEvents(r.ID)
	if !containsType(eventTypes(evs), EventLLMError) {
		t.Fatalf("expected llm.error skip event, got %v", eventTypes(evs))
	}
}
