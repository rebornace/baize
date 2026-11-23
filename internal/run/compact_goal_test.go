package run

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
)

func seedNestedGoalStack(t *testing.T, ms conversation.Store, convID string) conversation.GoalStack {
	t.Helper()
	stack := conversation.GoalStack{
		ConversationID: convID,
		Revision:       3,
		Frames: []conversation.GoalFrame{
			{ID: "g-root", Text: "交付 Active Goal v1", Status: conversation.GoalActive, Source: conversation.GoalSourceUser},
			{ID: "g-tip", Text: "写 Compactor 契约测试", Status: conversation.GoalActive, Source: conversation.GoalSourceUser},
		},
	}
	if err := ms.UpsertGoalStack(stack); err != nil {
		t.Fatal(err)
	}
	return stack
}

func goalFrameTexts(stack conversation.GoalStack) []string {
	out := make([]string, len(stack.Frames))
	for i, f := range stack.Frames {
		out[i] = f.Text
	}
	return out
}

func compactGoalFixture(t *testing.T) (*Compactor, *fakeCompactLLM, conversation.Store) {
	t.Helper()
	ms := conversation.NewMemoryStore()
	seedConv(t, ms, "c", 40)
	seedNestedGoalStack(t, ms, "c")
	llmF := &fakeCompactLLM{reply: "任意摘要"}
	c := &Compactor{
		Messages: ms, LLM: llmF,
		Profiles:  fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 4000}},
		Threshold: 0.7, ReserveTokens: 400, KeepRecent: 8,
	}
	return c, llmF, ms
}

func TestSummarizeIncludesActiveGoalBlock(t *testing.T) {
	c, llmF, ms := compactGoalFixture(t)
	if _, err := c.ForceCompact(context.Background(), "c", "p"); err != nil {
		t.Fatal(err)
	}
	if len(llmF.got) < 2 {
		t.Fatalf("expected summary prompt, got %d messages", len(llmF.got))
	}
	user := llmF.got[1].Content
	if !strings.Contains(user, "【系统 Active Goal") {
		t.Fatalf("missing Active Goal block: %q", user)
	}
	if !strings.Contains(user, "主目标：交付 Active Goal v1") || !strings.Contains(user, "当前焦点：写 Compactor 契约测试") {
		t.Fatalf("goal lines missing: %q", user)
	}
	if !strings.Contains(llmF.got[0].Content, "系统 Active Goal") {
		t.Fatal("system prompt must mention Active Goal alignment")
	}
	before, _ := ms.GetGoalStack("c")
	after, ok := ms.GetGoalStack("c")
	if !ok {
		t.Fatal("goal stack missing after compact")
	}
	if after.Revision != before.Revision || strings.Join(goalFrameTexts(after), "|") != strings.Join(goalFrameTexts(before), "|") {
		t.Fatalf("compact must not mutate goal stack: before=%+v after=%+v", before, after)
	}
}

func TestForceCompactPreservesGoalStack(t *testing.T) {
	c, _, ms := compactGoalFixture(t)
	before, _ := ms.GetGoalStack("c")
	changed, err := c.ForceCompact(context.Background(), "c", "p")
	if err != nil || !changed {
		t.Fatalf("ForceCompact: changed=%v err=%v", changed, err)
	}
	after, ok := ms.GetGoalStack("c")
	if !ok {
		t.Fatal("goal stack missing")
	}
	if after.Revision != before.Revision {
		t.Fatalf("revision changed: %d -> %d", before.Revision, after.Revision)
	}
	if strings.Join(goalFrameTexts(after), "|") != strings.Join(goalFrameTexts(before), "|") {
		t.Fatalf("frame texts changed: before=%v after=%v", goalFrameTexts(before), goalFrameTexts(after))
	}
}

func TestMaybeCompactPreservesGoalStack(t *testing.T) {
	c, _, ms := compactGoalFixture(t)
	before, _ := ms.GetGoalStack("c")
	changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
	if err != nil || !changed {
		t.Fatalf("MaybeCompact: changed=%v err=%v", changed, err)
	}
	after, ok := ms.GetGoalStack("c")
	if !ok {
		t.Fatal("goal stack missing")
	}
	if after.Revision != before.Revision || strings.Join(goalFrameTexts(after), "|") != strings.Join(goalFrameTexts(before), "|") {
		t.Fatalf("goal stack mutated: before=%+v after=%+v", before, after)
	}
}

func TestContextRewritePreservesGoalStack(t *testing.T) {
	ms := conversation.NewMemoryStore()
	seedNestedGoalStack(t, ms, "c1")
	e := &Engine{Messages: ms}
	before, _ := ms.GetGoalStack("c1")
	ctx := identity.WithConversationID(context.Background(), "c1")
	out, isErr, err := e.contextRewrite(ctx, map[string]any{"summary": "投影笔记"})
	if err != nil || isErr {
		t.Fatalf("rewrite: err=%v isErr=%v out=%v", err, isErr, out)
	}
	p, ok := ms.GetContextProjection("c1")
	if !ok || p.Summary != "投影笔记" {
		t.Fatalf("projection not updated: %+v ok=%v", p, ok)
	}
	after, ok := ms.GetGoalStack("c1")
	if !ok {
		t.Fatal("goal stack missing")
	}
	if after.Revision != before.Revision || strings.Join(goalFrameTexts(after), "|") != strings.Join(goalFrameTexts(before), "|") {
		t.Fatalf("rewrite touched goal stack: before=%+v after=%+v", before, after)
	}
}

func TestCompactSummaryGoalBlockSingleFrame(t *testing.T) {
	block := compactSummaryGoalBlock(conversation.GoalStack{
		Frames: []conversation.GoalFrame{{Text: "只做一件事", Status: conversation.GoalActive}},
	})
	if !strings.Contains(block, "主目标：只做一件事") || !strings.Contains(block, "当前焦点：只做一件事") {
		t.Fatalf("single frame block: %q", block)
	}
}
