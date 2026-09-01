package conversation

import "testing"

func TestMemoryRollingSummaryUpsertGetClear(t *testing.T) {
	s := NewMemoryStore()
	if _, ok := s.GetRollingSummary("c1"); ok {
		t.Fatal("expected no summary initially")
	}
	s.UpsertRollingSummary(RollingSummary{
		ConversationID:         "c1",
		Summary:                "用户在做报销项目",
		CoversThroughMessageID: "msg_10",
		CoversThroughOrder:     9,
	})
	got, ok := s.GetRollingSummary("c1")
	if !ok || got.Summary != "用户在做报销项目" || got.CoversThroughOrder != 9 {
		t.Fatalf("unexpected summary: %+v ok=%v", got, ok)
	}
	// 增量更新（同会话覆盖）
	s.UpsertRollingSummary(RollingSummary{
		ConversationID:         "c1",
		Summary:                "用户在做报销项目；已决定用 SQLite",
		CoversThroughMessageID: "msg_20",
		CoversThroughOrder:     19,
	})
	got, _ = s.GetRollingSummary("c1")
	if got.CoversThroughOrder != 19 || got.Summary == "用户在做报销项目" {
		t.Fatalf("update not applied: %+v", got)
	}
	// Clear 清空会话时联动删除摘要
	s.Append("c1", Message{Role: RoleUser, Content: "hi"})
	s.Clear("c1")
	if _, ok := s.GetRollingSummary("c1"); ok {
		t.Fatal("summary should be cleared with conversation")
	}
}

func TestMemoryRollingSummaryForkDoesNotCopy(t *testing.T) {
	s := NewMemoryStore()
	m1, _ := s.Append("c1", Message{Role: RoleUser, Content: "first"})
	s.Append("c1", Message{Role: RoleUser, Content: "second"})
	s.UpsertRollingSummary(RollingSummary{ConversationID: "c1", Summary: "S", CoversThroughMessageID: m1.ID, CoversThroughOrder: 0})
	newID, _, err := s.Fork("c1", m1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetRollingSummary(newID); ok {
		t.Fatal("forked conversation must not inherit rolling summary")
	}
}

func TestMemoryTruncateClearsSummary(t *testing.T) {
	s := NewMemoryStore()
	m1, _ := s.Append("c1", Message{Role: RoleUser, Content: "a"})
	s.Append("c1", Message{Role: RoleUser, Content: "b"})
	s.UpsertRollingSummary(RollingSummary{ConversationID: "c1", Summary: "S", CoversThroughMessageID: m1.ID, CoversThroughOrder: 0})
	if _, err := s.TruncateFrom("c1", m1.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetRollingSummary("c1"); ok {
		t.Fatal("truncate must clear rolling summary")
	}
}

func TestMemoryUpsertRollingSummaryRequiresConversationID(t *testing.T) {
	s := NewMemoryStore()
	if err := s.UpsertRollingSummary(RollingSummary{Summary: "x"}); err == nil {
		t.Fatal("empty ConversationID must return an error")
	}
}
