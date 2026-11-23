package conversation_test

import (
	"testing"

	"github.com/rebornace/baize/internal/conversation"
)

func TestSQLiteContextProjectionRoundTrip(t *testing.T) {
	s := newRollingSummaryStore(t)
	if _, ok := s.GetContextProjection("c1"); ok {
		t.Fatal("expected no projection")
	}
	if err := s.UpsertContextProjection(conversation.ContextProjection{
		ConversationID: "c1",
		Pins:           []string{"约束：必须走审批"},
		Summary:        "投影摘要",
		Dropped:        []string{"call_a"},
		Revision:       1,
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := s.GetContextProjection("c1")
	if !ok || got.Summary != "投影摘要" || len(got.Pins) != 1 || got.Pins[0] != "约束：必须走审批" ||
		len(got.Dropped) != 1 || got.Dropped[0] != "call_a" || got.Revision != 1 {
		t.Fatalf("round trip failed: %+v ok=%v", got, ok)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt should be set")
	}
}

func TestSQLiteTruncateClearsProjection(t *testing.T) {
	s := newRollingSummaryStore(t)
	m1, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertContextProjection(conversation.ContextProjection{
		ConversationID: "c1", Pins: []string{"p"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TruncateFrom("c1", m1.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetContextProjection("c1"); ok {
		t.Fatal("truncate must clear projection")
	}
}

func TestSQLiteClearClearsProjection(t *testing.T) {
	s := newRollingSummaryStore(t)
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertContextProjection(conversation.ContextProjection{ConversationID: "c1", Summary: "s"}); err != nil {
		t.Fatal(err)
	}
	s.Clear("c1")
	if _, ok := s.GetContextProjection("c1"); ok {
		t.Fatal("clear must drop projection")
	}
}

func TestSQLiteUpsertContextProjectionRequiresConversationID(t *testing.T) {
	s := newRollingSummaryStore(t)
	if err := s.UpsertContextProjection(conversation.ContextProjection{Summary: "x"}); err == nil {
		t.Fatal("expected error")
	}
}
