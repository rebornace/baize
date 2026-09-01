package conversation_test

import (
	"testing"

	"github.com/rebornace/baize/internal/conversation"
)

func newRollingSummaryStore(t *testing.T) *conversation.SQLiteStore {
	t.Helper()
	db, _ := openTestDB(t)
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSQLiteRollingSummaryRoundTrip(t *testing.T) {
	s := newRollingSummaryStore(t)
	if _, ok := s.GetRollingSummary("c1"); ok {
		t.Fatal("expected no summary")
	}
	if err := s.UpsertRollingSummary(conversation.RollingSummary{
		ConversationID:         "c1",
		Summary:                "旧对话摘要",
		CoversThroughMessageID: "msg_5",
		CoversThroughOrder:     4,
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := s.GetRollingSummary("c1")
	if !ok || got.Summary != "旧对话摘要" || got.CoversThroughOrder != 4 || got.CoversThroughMessageID != "msg_5" {
		t.Fatalf("round trip failed: %+v ok=%v", got, ok)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt should be auto-populated (UTC now) and round-tripped")
	}

	// Incremental update (same conversation overwrites the row).
	if err := s.UpsertRollingSummary(conversation.RollingSummary{
		ConversationID:         "c1",
		Summary:                "新摘要",
		CoversThroughMessageID: "msg_9",
		CoversThroughOrder:     8,
	}); err != nil {
		t.Fatal(err)
	}
	got, ok = s.GetRollingSummary("c1")
	if !ok || got.Summary != "新摘要" || got.CoversThroughOrder != 8 || got.CoversThroughMessageID != "msg_9" {
		t.Fatalf("update not applied: %+v ok=%v", got, ok)
	}
}

func TestSQLiteTruncateClearsSummary(t *testing.T) {
	s := newRollingSummaryStore(t)
	m1, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "a"})
	if err != nil {
		t.Fatal(err)
	}
	m2, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertRollingSummary(conversation.RollingSummary{
		ConversationID:         "c1",
		Summary:                "S",
		CoversThroughMessageID: m2.ID,
		CoversThroughOrder:     1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TruncateFrom("c1", m1.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetRollingSummary("c1"); ok {
		t.Fatal("truncate must clear rolling summary")
	}
}

func TestSQLiteClearClearsSummary(t *testing.T) {
	s := newRollingSummaryStore(t)
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertRollingSummary(conversation.RollingSummary{
		ConversationID:         "c1",
		Summary:                "S",
		CoversThroughMessageID: "msg_x",
		CoversThroughOrder:     0,
	}); err != nil {
		t.Fatal(err)
	}
	s.Clear("c1")
	if _, ok := s.GetRollingSummary("c1"); ok {
		t.Fatal("clear must clear rolling summary")
	}
}

func TestSQLiteUpsertRollingSummaryRequiresConversationID(t *testing.T) {
	s := newRollingSummaryStore(t)
	if err := s.UpsertRollingSummary(conversation.RollingSummary{Summary: "x"}); err == nil {
		t.Fatal("empty ConversationID must return an error")
	}
}
