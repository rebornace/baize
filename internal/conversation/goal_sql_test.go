package conversation_test

import (
	"testing"

	"github.com/rebornace/baize/internal/conversation"
)

func TestSQLiteGoalStackRoundTrip(t *testing.T) {
	s := newRollingSummaryStore(t)
	if _, ok := s.GetGoalStack("c1"); ok {
		t.Fatal("expected no goal stack")
	}
	if err := s.UpsertGoalStack(conversation.GoalStack{
		ConversationID: "c1",
		Frames: []conversation.GoalFrame{{
			ID: "g1", Text: "实现登录", Status: conversation.GoalActive, Source: conversation.GoalSourceUser,
		}},
		Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := s.GetGoalStack("c1")
	if !ok || len(got.Frames) != 1 || got.Frames[0].Text != "实现登录" || got.Revision != 1 {
		t.Fatalf("round trip failed: %+v ok=%v", got, ok)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt should be set")
	}
}

func TestSQLiteTruncateClearsGoalStack(t *testing.T) {
	s := newRollingSummaryStore(t)
	m1, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertGoalStack(conversation.GoalStack{
		ConversationID: "c1",
		Frames:         []conversation.GoalFrame{{ID: "g1", Text: "目标", Status: conversation.GoalActive, Source: conversation.GoalSourceUser}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TruncateFrom("c1", m1.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetGoalStack("c1"); ok {
		t.Fatal("truncate must clear goal stack")
	}
}

func TestSQLiteClearClearsGoalStack(t *testing.T) {
	s := newRollingSummaryStore(t)
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertGoalStack(conversation.GoalStack{
		ConversationID: "c1",
		Frames:         []conversation.GoalFrame{{ID: "g1", Text: "目标", Status: conversation.GoalActive, Source: conversation.GoalSourceUser}},
	}); err != nil {
		t.Fatal(err)
	}
	s.Clear("c1")
	if _, ok := s.GetGoalStack("c1"); ok {
		t.Fatal("clear must drop goal stack")
	}
}

func TestSQLiteUpsertGoalStackRequiresConversationID(t *testing.T) {
	s := newRollingSummaryStore(t)
	if err := s.UpsertGoalStack(conversation.GoalStack{
		Frames: []conversation.GoalFrame{{ID: "g1", Text: "x", Status: conversation.GoalActive, Source: conversation.GoalSourceUser}},
	}); err == nil {
		t.Fatal("expected error")
	}
}

func TestSQLiteForkCopiesGoalStack(t *testing.T) {
	s := newRollingSummaryStore(t)
	m1, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "second"}); err != nil {
		t.Fatal(err)
	}
	srcID := "g-src"
	if err := s.UpsertGoalStack(conversation.GoalStack{
		ConversationID: "c1",
		Frames:         []conversation.GoalFrame{{ID: srcID, Text: "主目标", Status: conversation.GoalActive, Source: conversation.GoalSourceUser}},
	}); err != nil {
		t.Fatal(err)
	}
	newID, _, err := s.Fork("c1", m1.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s.GetGoalStack(newID)
	if !ok || len(got.Frames) != 1 || got.Frames[0].Text != "主目标" {
		t.Fatalf("fork copy: %+v ok=%v", got, ok)
	}
	if got.Frames[0].ID == srcID {
		t.Fatal("fork must assign new frame ids")
	}
}
