package run

import (
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
)

func TestBuildMessagesInjectsActiveGoalBeforeHistory(t *testing.T) {
	ms := conversation.NewMemoryStore()
	_, _ = ms.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "旧消息"})
	_ = ms.UpsertGoalStack(conversation.GoalStack{
		ConversationID: "c1",
		Frames: []conversation.GoalFrame{{
			ID: "g1", Text: "实现退款", Status: conversation.GoalActive, Source: conversation.GoalSourceUser,
		}},
	})
	e := &Engine{
		Messages: ms,
		Settings: fakeKnobs{k: runtimecfg.Knobs{
			ActiveGoalEnabled: true, MaxMessages: 50,
		}},
	}
	msgs := e.buildMessages("系统", "c1", "现在问", nil)
	var goalIdx, histIdx = -1, -1
	for i, m := range msgs {
		if strings.Contains(m.Content, "【当前目标】") || strings.Contains(m.Content, "【主目标】") {
			goalIdx = i
		}
		if m.Role == llm.RoleUser && m.Content == "旧消息" {
			histIdx = i
		}
	}
	if goalIdx < 0 || histIdx < 0 || goalIdx >= histIdx {
		t.Fatalf("goalIdx=%d histIdx=%d msgs=%+v", goalIdx, histIdx, msgs)
	}
}

func TestBuildMessagesNoGoalWhenDisabled(t *testing.T) {
	ms := conversation.NewMemoryStore()
	_ = ms.UpsertGoalStack(conversation.GoalStack{
		ConversationID: "c1",
		Frames: []conversation.GoalFrame{{
			ID: "g1", Text: "实现退款", Status: conversation.GoalActive, Source: conversation.GoalSourceUser,
		}},
	})
	e := &Engine{
		Messages: ms,
		Settings: fakeKnobs{k: runtimecfg.Knobs{ActiveGoalEnabled: false, MaxMessages: 50}},
	}
	msgs := e.buildMessages("系统", "c1", "问", nil)
	for _, m := range msgs {
		if strings.Contains(m.Content, "【当前目标】") || strings.Contains(m.Content, "【主目标】") {
			t.Fatalf("disabled must not inject goal: %+v", msgs)
		}
	}
}

func TestGoalBlockAllDoneNoInject(t *testing.T) {
	stack := conversation.GoalStack{
		ConversationID: "c1",
		Frames: []conversation.GoalFrame{{
			ID: "g1", Text: "已完成", Status: conversation.GoalDone, Source: conversation.GoalSourceUser,
		}},
	}
	if block := goalBlock(stack); block != "" {
		t.Fatalf("all done must not produce block: %q", block)
	}
	ms := conversation.NewMemoryStore()
	_ = ms.UpsertGoalStack(stack)
	e := &Engine{
		Messages: ms,
		Settings: fakeKnobs{k: runtimecfg.Knobs{ActiveGoalEnabled: true, MaxMessages: 50}},
	}
	for _, m := range e.buildMessages("系统", "c1", "问", nil) {
		if strings.Contains(m.Content, "【主目标】") || strings.Contains(m.Content, "【当前目标】") {
			t.Fatalf("all done stack must not inject: %+v", m)
		}
	}
}

func TestBuildMessagesInjectsPausedWhenInterruptedOnly(t *testing.T) {
	ms := conversation.NewMemoryStore()
	stopped, _ := conversation.SetRoot(conversation.GoalStack{}, "c1", "仅主", nil, conversation.GoalSourceUser)
	stopped, _ = conversation.Stop(stopped)
	_ = ms.UpsertGoalStack(stopped)
	e := &Engine{
		Messages: ms,
		Settings: fakeKnobs{k: runtimecfg.Knobs{ActiveGoalEnabled: true, MaxMessages: 50}},
	}
	msgs := e.buildMessages("系统", "c1", "继续吗", nil)
	var found bool
	for _, m := range msgs {
		if m.Content == "【主目标】仅主（已暂停，可继续）" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected paused goal block, msgs=%+v", msgs)
	}
}

func TestGoalBlockNestedAndInterrupted(t *testing.T) {
	rootTip, _ := conversation.SetRoot(conversation.GoalStack{}, "c1", "主任务", nil, conversation.GoalSourceUser)
	rootTip, _ = conversation.Push(rootTip, "子焦点", []string{"完成 A"}, conversation.GoalSourceUser, 8)
	block := goalBlock(rootTip)
	if !strings.Contains(block, "【主目标】主任务") || !strings.Contains(block, "【当前焦点】子焦点") || !strings.Contains(block, "【验收】") {
		t.Fatalf("nested block=%q", block)
	}
	stopped, _ := conversation.SetRoot(conversation.GoalStack{}, "c1", "仅主", nil, conversation.GoalSourceUser)
	stopped, _ = conversation.Stop(stopped)
	block = goalBlock(stopped)
	if block != "【主目标】仅主（已暂停，可继续）" {
		t.Fatalf("interrupted block=%q", block)
	}
	nestedStop, _ := conversation.SetRoot(conversation.GoalStack{}, "c1", "主任务", nil, conversation.GoalSourceUser)
	nestedStop, _ = conversation.Push(nestedStop, "子焦点", nil, conversation.GoalSourceUser, 8)
	nestedStop, _ = conversation.Stop(nestedStop)
	block = goalBlock(nestedStop)
	if !strings.Contains(block, "【主目标】主任务") || !strings.Contains(block, "【已暂停焦点】子焦点") {
		t.Fatalf("nested stop block=%q", block)
	}
}

func TestMaybeSeedGoalHarness(t *testing.T) {
	ms := conversation.NewMemoryStore()
	mem := store.NewMemory()
	r, err := mem.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x", ConversationID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	e := &Engine{
		Messages: ms,
		Store:    mem,
		Settings: fakeKnobs{k: runtimecfg.Knobs{
			ActiveGoalEnabled: true, ActiveGoalMaxTextRunes: 10,
		}},
	}
	long := strings.Repeat("字", 20)
	e.maybeSeedGoal(r.ID, "c1", long)
	stack, ok := ms.GetGoalStack("c1")
	if !ok || len(stack.Frames) != 1 {
		t.Fatalf("stack=%+v ok=%v", stack, ok)
	}
	if stack.Frames[0].Source != conversation.GoalSourceHarness {
		t.Fatalf("source=%q", stack.Frames[0].Source)
	}
	if got := []rune(stack.Frames[0].Text); len(got) != 10 {
		t.Fatalf("truncated len=%d text=%q", len(got), stack.Frames[0].Text)
	}
	evs, err := mem.ListEvents(r.ID)
	if err != nil || len(evs) != 1 || evs[0].Type != EventGoalSeeded {
		t.Fatalf("events=%+v err=%v", evs, err)
	}
}

func TestMaybeSeedGoalSkipsNonemptyStack(t *testing.T) {
	ms := conversation.NewMemoryStore()
	_ = ms.UpsertGoalStack(conversation.GoalStack{
		ConversationID: "c1",
		Frames: []conversation.GoalFrame{{
			ID: "g1", Text: "已有", Status: conversation.GoalActive, Source: conversation.GoalSourceUser,
			UpdatedAt: time.Now().UTC(),
		}},
	})
	e := &Engine{
		Messages: ms,
		Settings: fakeKnobs{k: runtimecfg.Knobs{ActiveGoalEnabled: true}},
	}
	e.maybeSeedGoal("run1", "c1", "新输入")
	stack, _ := ms.GetGoalStack("c1")
	if stack.Frames[0].Text != "已有" {
		t.Fatalf("stack overwritten: %+v", stack)
	}
}
