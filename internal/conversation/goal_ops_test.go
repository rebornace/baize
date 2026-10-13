package conversation_test

import (
	"testing"

	"github.com/rebornace/baize/internal/conversation"
)

func TestGoalPushPopRestoresRoot(t *testing.T) {
	var s conversation.GoalStack
	s, err := conversation.SetRoot(s, "c1", "实现登录", nil, conversation.GoalSourceUser)
	if err != nil {
		t.Fatal(err)
	}
	s, err = conversation.Push(s, "先查一下现有 auth", nil, conversation.GoalSourceUser, 8)
	if err != nil {
		t.Fatal(err)
	}
	if tip, ok := conversation.Tip(s); !ok || tip.Text != "先查一下现有 auth" {
		t.Fatalf("tip=%+v", tip)
	}
	s, err = conversation.Pop(s)
	if err != nil {
		t.Fatal(err)
	}
	if tip, ok := conversation.Tip(s); !ok || tip.Text != "实现登录" {
		t.Fatalf("after pop tip=%+v ok=%v", tip, ok)
	}
}

func TestGoalStopResume(t *testing.T) {
	s, _ := conversation.SetRoot(conversation.GoalStack{}, "c1", "主任务", nil, conversation.GoalSourceUser)
	s, err := conversation.Stop(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := conversation.Tip(s); ok {
		t.Fatal("no active tip while interrupted")
	}
	s, err = conversation.Resume(s)
	if err != nil {
		t.Fatal(err)
	}
	tip, ok := conversation.Tip(s)
	if !ok || tip.Text != "主任务" || tip.Status != conversation.GoalActive {
		t.Fatalf("resumed=%+v", tip)
	}
}

func TestGoalStopPopNestedDiscardsInterrupted(t *testing.T) {
	s, err := conversation.SetRoot(conversation.GoalStack{}, "c1", "实现登录", nil, conversation.GoalSourceUser)
	if err != nil {
		t.Fatal(err)
	}
	s, err = conversation.Push(s, "先查 auth", nil, conversation.GoalSourceUser, 8)
	if err != nil {
		t.Fatal(err)
	}
	s, err = conversation.Stop(s)
	if err != nil {
		t.Fatal(err)
	}
	s, err = conversation.Pop(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Frames) != 1 {
		t.Fatalf("frames=%+v", s.Frames)
	}
	if conversation.HasInterruptedFrame(s) {
		t.Fatal("interrupted frame should be gone")
	}
	tip, ok := conversation.Tip(s)
	if !ok || tip.Text != "实现登录" || tip.Status != conversation.GoalActive {
		t.Fatalf("tip=%+v ok=%v", tip, ok)
	}
}

func TestGoalStopResumeNestedFocus(t *testing.T) {
	s, err := conversation.SetRoot(conversation.GoalStack{}, "c1", "实现登录", nil, conversation.GoalSourceUser)
	if err != nil {
		t.Fatal(err)
	}
	s, err = conversation.Push(s, "先查 auth", nil, conversation.GoalSourceUser, 8)
	if err != nil {
		t.Fatal(err)
	}
	s, err = conversation.Stop(s)
	if err != nil {
		t.Fatal(err)
	}
	tip, ok := conversation.Tip(s)
	if !ok || tip.Text != "实现登录" || tip.Status != conversation.GoalActive {
		t.Fatalf("tip after stop=%+v ok=%v", tip, ok)
	}
	top := s.Frames[len(s.Frames)-1]
	if top.Text != "先查 auth" || top.Status != conversation.GoalInterrupted {
		t.Fatalf("top frame=%+v", top)
	}
	s, err = conversation.Resume(s)
	if err != nil {
		t.Fatal(err)
	}
	tip, ok = conversation.Tip(s)
	if !ok || tip.Text != "先查 auth" || tip.Status != conversation.GoalActive {
		t.Fatalf("tip after resume=%+v ok=%v", tip, ok)
	}
}

func TestGoalPatchTip(t *testing.T) {
	criteria := []string{"JWT", "OAuth"}
	s, err := conversation.SetRoot(conversation.GoalStack{}, "c1", "原目标", criteria, conversation.GoalSourceUser)
	if err != nil {
		t.Fatal(err)
	}

	s, err = conversation.PatchTip(s, "新目标", nil)
	if err != nil {
		t.Fatal(err)
	}
	tip, ok := conversation.Tip(s)
	if !ok || tip.Text != "新目标" || len(tip.Criteria) != 2 || tip.Criteria[0] != "JWT" {
		t.Fatalf("text-only patch=%+v", tip)
	}

	s, err = conversation.PatchTip(s, "", []string{})
	if err != nil {
		t.Fatal(err)
	}
	tip, ok = conversation.Tip(s)
	if !ok || tip.Text != "新目标" || len(tip.Criteria) != 0 {
		t.Fatalf("clear criteria=%+v", tip)
	}

	s, err = conversation.PatchTip(s, "终稿", []string{"单测通过"})
	if err != nil {
		t.Fatal(err)
	}
	tip, ok = conversation.Tip(s)
	if !ok || tip.Text != "终稿" || len(tip.Criteria) != 1 || tip.Criteria[0] != "单测通过" {
		t.Fatalf("both patch=%+v", tip)
	}
}

func TestGoalPushRespectsMaxDepth(t *testing.T) {
	s, _ := conversation.SetRoot(conversation.GoalStack{}, "c1", "r", nil, conversation.GoalSourceUser)
	var err error
	for i := 0; i < 7; i++ {
		s, err = conversation.Push(s, "x", nil, conversation.GoalSourceUser, 8)
		if err != nil {
			t.Fatalf("push %d: %v", i, err)
		}
	}
	if _, err = conversation.Push(s, "overflow", nil, conversation.GoalSourceUser, 8); err == nil {
		t.Fatal("want depth error")
	}
}
