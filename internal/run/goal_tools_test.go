package run

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func goalToolEngine(t *testing.T, enabled bool) (*Engine, *conversation.MemoryStore, store.Store) {
	t.Helper()
	ms := conversation.NewMemoryStore()
	st := store.NewMemory()
	e := &Engine{
		Messages: ms,
		Store:    st,
		Settings: fakeKnobs{k: runtimecfg.Knobs{
			ActiveGoalEnabled:      enabled,
			ActiveGoalMaxDepth:     8,
			ActiveGoalMaxTextRunes: 500,
			MaxMessages:            50,
		}},
	}
	return e, ms, st
}

func goalToolCtx(runID, convID string) context.Context {
	ctx := identity.WithConversationID(context.Background(), convID)
	return identity.WithRunID(ctx, runID)
}

func TestGoalSetPushCompleteUpdate(t *testing.T) {
	e, ms, st := goalToolEngine(t, true)
	r, err := st.CreateRun(store.CreateRunInput{ConversationID: "c1", AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := goalToolCtx(r.ID, "c1")

	out, isErr, err := e.goalSet(ctx, map[string]any{"text": "实现登录", "criteria": []any{"单测通过"}})
	if err != nil || isErr {
		t.Fatalf("set: %v isErr=%v out=%v", err, isErr, out)
	}
	if out["root"] != "实现登录" || out["depth"].(int) != 1 {
		t.Fatalf("set out=%v", out)
	}

	out, isErr, err = e.goalPush(ctx, map[string]any{"text": "先查 auth"})
	if err != nil || isErr {
		t.Fatalf("push: %v isErr=%v out=%v", err, isErr, out)
	}
	if out["tip"] != "先查 auth" || out["depth"].(int) != 2 {
		t.Fatalf("push out=%v", out)
	}

	out, isErr, err = e.goalUpdate(ctx, map[string]any{"text": "查 OAuth 配置"})
	if err != nil || isErr {
		t.Fatalf("update: %v isErr=%v out=%v", err, isErr, out)
	}
	if out["tip"] != "查 OAuth 配置" {
		t.Fatalf("update out=%v", out)
	}

	out, isErr, err = e.goalComplete(ctx, nil)
	if err != nil || isErr {
		t.Fatalf("complete: %v isErr=%v out=%v", err, isErr, out)
	}
	if out["tip"] != "实现登录" || out["depth"].(int) != 1 {
		t.Fatalf("complete out=%v", out)
	}

	stack, ok := ms.GetGoalStack("c1")
	if !ok || len(stack.Frames) != 1 || stack.Frames[0].Source != conversation.GoalSourceAgent {
		t.Fatalf("stack=%+v ok=%v", stack, ok)
	}
	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, ev := range evs {
		seen[ev.Type] = true
	}
	for _, typ := range []string{EventGoalSet, EventGoalPushed, EventGoalUpdated, EventGoalPopped} {
		if !seen[typ] {
			t.Fatalf("missing event %s in %+v", typ, evs)
		}
	}
	if got := ms.List("c1"); len(got) != 0 {
		t.Fatalf("messages must stay empty: %+v", got)
	}
}

func TestGoalTurnBudgetPauses(t *testing.T) {
	e, ms, st := goalToolEngine(t, true)
	r, err := st.CreateRun(store.CreateRunInput{ConversationID: "c1", AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := goalToolCtx(r.ID, "c1")
	if _, isErr, err := e.goalSet(ctx, map[string]any{"text": "x", "max_turns": 2}); err != nil || isErr {
		t.Fatalf("set: %v isErr=%v", err, isErr)
	}
	stack, _ := ms.GetGoalStack("c1")
	if tip, _ := conversation.Tip(stack); tip.MaxTurns != 2 || tip.UsedTurns != 0 {
		t.Fatalf("tip=%+v", tip)
	}
	e.incrementGoalTurn("c1")
	e.incrementGoalTurn("c1")
	paused, note := e.goalBudgetExceeded("c1")
	if !paused || !strings.Contains(note, "2/2") {
		t.Fatalf("paused=%v note=%q", paused, note)
	}
	stack, _ = ms.GetGoalStack("c1")
	tip, ok := conversation.InterruptedTip(stack)
	if !ok || tip.Status != conversation.GoalInterrupted {
		t.Fatalf("tip=%+v ok=%v want interrupted", tip, ok)
	}
}

func TestGoalTokenBudgetPauses(t *testing.T) {
	e, ms, st := goalToolEngine(t, true)
	r, err := st.CreateRun(store.CreateRunInput{ConversationID: "c1", AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := goalToolCtx(r.ID, "c1")
	if _, isErr, err := e.goalSet(ctx, map[string]any{"text": "x", "max_tokens": 100}); err != nil || isErr {
		t.Fatalf("set: %v isErr=%v", err, isErr)
	}
	stack, _ := ms.GetGoalStack("c1")
	if tip, _ := conversation.Tip(stack); tip.MaxTokens != 100 || tip.UsedTokens != 0 {
		t.Fatalf("tip=%+v", tip)
	}
	e.addGoalTokens("c1", 60)
	e.addGoalTokens("c1", 50)
	paused, note := e.goalBudgetExceeded("c1")
	if !paused || !strings.Contains(note, "token budget") || !strings.Contains(note, "110/100") {
		t.Fatalf("paused=%v note=%q", paused, note)
	}
	stack, _ = ms.GetGoalStack("c1")
	tip, ok := conversation.InterruptedTip(stack)
	if !ok || tip.Status != conversation.GoalInterrupted || tip.UsedTokens != 110 {
		t.Fatalf("tip=%+v ok=%v want interrupted", tip, ok)
	}
}

func TestGoalTokenBudgetValidation(t *testing.T) {
	e, _, st := goalToolEngine(t, true)
	r, err := st.CreateRun(store.CreateRunInput{ConversationID: "c1", AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := goalToolCtx(r.ID, "c1")
	if _, isErr, err := e.goalSet(ctx, map[string]any{"text": "x", "max_tokens": -1}); err != nil || !isErr {
		t.Fatalf("set negative: err=%v isErr=%v", err, isErr)
	}
	if _, isErr, err := e.goalSet(ctx, map[string]any{"text": "x", "max_tokens": conversation.MaxGoalTokenBudget + 1}); err != nil || !isErr {
		t.Fatalf("set over cap: err=%v isErr=%v", err, isErr)
	}
}

func TestGoalCompleteRequiresEvidenceForCriteria(t *testing.T) {
	e, ms, st := goalToolEngine(t, true)
	r, err := st.CreateRun(store.CreateRunInput{ConversationID: "c1", AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := goalToolCtx(r.ID, "c1")
	if _, isErr, err := e.goalSet(ctx, map[string]any{"text": "上线", "criteria": []any{"测试通过"}}); err != nil || isErr {
		t.Fatalf("set: %v isErr=%v", err, isErr)
	}
	out, isErr, err := e.goalComplete(ctx, map[string]any{})
	if err != nil || !isErr || !strings.Contains(fmt.Sprint(out["error"]), "evidence") {
		t.Fatalf("missing evidence must fail: out=%v err=%v isErr=%v", out, err, isErr)
	}
	if stack, ok := ms.GetGoalStack("c1"); !ok || len(stack.Frames) != 1 {
		t.Fatalf("stack changed after rejected completion: %+v", stack)
	}
	out, isErr, err = e.goalComplete(ctx, map[string]any{"evidence": "CI 全量通过"})
	if err != nil || isErr {
		t.Fatalf("complete: out=%v err=%v isErr=%v", out, err, isErr)
	}
	if out["depth"].(int) != 0 {
		t.Fatalf("stack should be empty: %v", out)
	}
	evs, _ := st.ListEvents(r.ID)
	found := false
	for _, ev := range evs {
		if ev.Type == EventGoalCompleted {
			found = true
			if ev.Data["evidence"] != "CI 全量通过" {
				t.Fatalf("completed event=%+v", ev.Data)
			}
		}
	}
	if !found {
		t.Fatalf("missing %s", EventGoalCompleted)
	}
}

func TestGoalPushRequiresRoot(t *testing.T) {
	e, _, st := goalToolEngine(t, true)
	r, err := st.CreateRun(store.CreateRunInput{ConversationID: "c1", AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	_, isErr, err := e.goalPush(goalToolCtx(r.ID, "c1"), map[string]any{"text": "临时"})
	if err != nil || !isErr {
		t.Fatalf("want isErr, got err=%v isErr=%v", err, isErr)
	}
}

func TestGoalToolsDisabled(t *testing.T) {
	e, _, st := goalToolEngine(t, false)
	r, err := st.CreateRun(store.CreateRunInput{ConversationID: "c1", AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	_, isErr, err := e.goalSet(goalToolCtx(r.ID, "c1"), map[string]any{"text": "x"})
	if err != nil || !isErr {
		t.Fatalf("disabled set want isErr")
	}
}

func TestSpecsForRunIncludesGoalToolsWhenEnabled(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register("echo", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	e := &Engine{
		Tools:    reg,
		Settings: fakeKnobs{k: runtimecfg.Knobs{ActiveGoalEnabled: true}},
	}
	for _, tm := range e.GoalTools() {
		reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
	}
	e.beginRunSkills("r1", nil, "sys")
	specs := e.specsForRun("r1")
	seen := map[string]bool{}
	for _, s := range specs {
		seen[s.Name] = true
	}
	for name := range goalToolNames {
		if !seen[name] {
			t.Fatalf("enabled must include %s", name)
		}
	}

	e.Settings = fakeKnobs{k: runtimecfg.Knobs{ActiveGoalEnabled: false}}
	specs = e.specsForRun("r1")
	for _, s := range specs {
		if goalToolNames[s.Name] {
			t.Fatalf("disabled must hide %s", s.Name)
		}
	}
}

func TestRefreshActiveGoalMessageReplacesBlock(t *testing.T) {
	e, ms, _ := goalToolEngine(t, true)
	_ = ms.UpsertGoalStack(conversation.GoalStack{
		ConversationID: "c1",
		Frames: []conversation.GoalFrame{{
			ID: "g1", Text: "旧目标", Status: conversation.GoalActive, Source: conversation.GoalSourceUser,
		}},
	})
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "系统"},
		{Role: llm.RoleSystem, Content: "【当前目标】旧目标"},
		{Role: llm.RoleUser, Content: "继续"},
	}
	_ = ms.UpsertGoalStack(conversation.GoalStack{
		ConversationID: "c1",
		Revision:       2,
		Frames: []conversation.GoalFrame{{
			ID: "g2", Text: "新目标", Status: conversation.GoalActive, Source: conversation.GoalSourceAgent,
		}},
	})
	got := e.refreshActiveGoalMessage(msgs, "c1")
	var goalCount int
	for _, m := range got {
		if m.Role == llm.RoleSystem && isGoalSystemContent(m.Content) {
			goalCount++
			if !strings.Contains(m.Content, "新目标") {
				t.Fatalf("want new goal, got %q", m.Content)
			}
		}
	}
	if goalCount != 1 {
		t.Fatalf("goalCount=%d msgs=%+v", goalCount, got)
	}
}

func TestRefreshActiveGoalMessageInsertsWhenMissing(t *testing.T) {
	e, ms, _ := goalToolEngine(t, true)
	_ = ms.UpsertGoalStack(conversation.GoalStack{
		ConversationID: "c1",
		Frames: []conversation.GoalFrame{{
			ID: "g1", Text: "植入", Status: conversation.GoalActive, Source: conversation.GoalSourceAgent,
		}},
	})
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "系统"},
		{Role: llm.RoleUser, Content: "问"},
	}
	got := e.refreshActiveGoalMessage(msgs, "c1")
	if len(got) < 3 || !isGoalSystemContent(got[1].Content) {
		t.Fatalf("expected insert after system: %+v", got)
	}
}

func TestGoalSetMidRunVisibleOnNextRefresh(t *testing.T) {
	e, _, st := goalToolEngine(t, true)
	r, err := st.CreateRun(store.CreateRunInput{ConversationID: "c1", AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := goalToolCtx(r.ID, "c1")
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "系统"},
		{Role: llm.RoleUser, Content: "开始"},
	}
	msgs = e.refreshActiveGoalMessage(msgs, "c1")
	for _, m := range msgs {
		if isGoalSystemContent(m.Content) {
			t.Fatalf("empty stack must not inject: %+v", msgs)
		}
	}
	if _, isErr, err := e.goalSet(ctx, map[string]any{"text": "修好退款"}); err != nil || isErr {
		t.Fatalf("set: %v isErr=%v", err, isErr)
	}
	msgs = e.refreshActiveGoalMessage(msgs, "c1")
	found := false
	for _, m := range msgs {
		if strings.Contains(m.Content, "修好退款") {
			found = true
		}
	}
	if !found {
		t.Fatalf("refresh after set must show goal: %+v", msgs)
	}
}

func TestGoalToolsRejectChildRuns(t *testing.T) {
	e, ms, st := goalToolEngine(t, true)
	parent, err := st.CreateRun(store.CreateRunInput{ConversationID: "c1", AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := st.CreateRun(store.CreateRunInput{
		ConversationID: "c1", AgentID: "a", Input: "sub", ParentRunID: parent.ID, Depth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := goalToolCtx(child.ID, "c1")
	out, isErr, err := e.goalSet(ctx, map[string]any{"text": "child goal"})
	if err != nil || !isErr {
		t.Fatalf("child set: err=%v isErr=%v", err, isErr)
	}
	if !strings.Contains(fmt.Sprint(out["error"]), "child runs") {
		t.Fatalf("out=%v", out)
	}
	if _, ok := ms.GetGoalStack("c1"); ok {
		t.Fatal("child run must not write the goal stack")
	}

	pctx := goalToolCtx(parent.ID, "c1")
	if _, isErr, err := e.goalSet(pctx, map[string]any{"text": "parent goal"}); err != nil || isErr {
		t.Fatalf("parent set: err=%v isErr=%v", err, isErr)
	}
	if stack, ok := ms.GetGoalStack("c1"); !ok || len(stack.Frames) != 1 {
		t.Fatalf("parent stack=%+v ok=%v", stack, ok)
	}
}
