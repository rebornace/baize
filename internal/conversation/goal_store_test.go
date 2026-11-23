package conversation

import "testing"

func TestGoalUpsertDoesNotTouchMessages(t *testing.T) {
	s := NewMemoryStore()
	m, _ := s.Append("c1", Message{Role: RoleUser, Content: "hello"})
	_ = s.UpsertGoalStack(GoalStack{ConversationID: "c1", Frames: []GoalFrame{{
		ID: "g1", Text: "目标", Status: GoalActive, Source: GoalSourceUser,
	}}})
	list := s.List("c1")
	if len(list) != 1 || list[0].ID != m.ID || list[0].Content != "hello" {
		t.Fatalf("messages mutated: %+v", list)
	}
}

func TestMemoryGoalStackRoundTrip(t *testing.T) {
	s := NewMemoryStore()
	if _, ok := s.GetGoalStack("c1"); ok {
		t.Fatal("expected no goal stack")
	}
	if err := s.UpsertGoalStack(GoalStack{
		ConversationID: "c1",
		Frames: []GoalFrame{{
			ID: "g1", Text: "实现登录", Status: GoalActive, Source: GoalSourceUser, Revision: 1,
		}},
		Revision: 2,
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := s.GetGoalStack("c1")
	if !ok || len(got.Frames) != 1 || got.Frames[0].Text != "实现登录" || got.Revision != 2 {
		t.Fatalf("round trip failed: %+v ok=%v", got, ok)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt should be set")
	}
}

func TestMemoryTruncateClearsGoalStack(t *testing.T) {
	s := NewMemoryStore()
	m1, err := s.Append("c1", Message{Role: RoleUser, Content: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c1", Message{Role: RoleUser, Content: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertGoalStack(GoalStack{
		ConversationID: "c1",
		Frames:         []GoalFrame{{ID: "g1", Text: "目标", Status: GoalActive, Source: GoalSourceUser}},
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

func TestMemoryClearClearsGoalStack(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.Append("c1", Message{Role: RoleUser, Content: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertGoalStack(GoalStack{
		ConversationID: "c1",
		Frames:         []GoalFrame{{ID: "g1", Text: "目标", Status: GoalActive, Source: GoalSourceUser}},
	}); err != nil {
		t.Fatal(err)
	}
	s.Clear("c1")
	if _, ok := s.GetGoalStack("c1"); ok {
		t.Fatal("clear must drop goal stack")
	}
}

func TestMemoryUpsertGoalStackRequiresConversationID(t *testing.T) {
	s := NewMemoryStore()
	if err := s.UpsertGoalStack(GoalStack{
		Frames: []GoalFrame{{ID: "g1", Text: "x", Status: GoalActive, Source: GoalSourceUser}},
	}); err == nil {
		t.Fatal("expected error")
	}
}

func TestForkCopiesGoalStack(t *testing.T) {
	s := NewMemoryStore()
	m1, err := s.Append("c1", Message{Role: RoleUser, Content: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c1", Message{Role: RoleUser, Content: "second"}); err != nil {
		t.Fatal(err)
	}
	srcFrameID := "frame-src-1"
	if err := s.UpsertGoalStack(GoalStack{
		ConversationID: "c1",
		Frames: []GoalFrame{{
			ID: srcFrameID, Text: "主目标", Status: GoalActive, Source: GoalSourceUser, Revision: 3,
		}},
		Revision: 5,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertRollingSummary(RollingSummary{
		ConversationID: "c1", Summary: "S", CoversThroughMessageID: m1.ID, CoversThroughOrder: 0,
	}); err != nil {
		t.Fatal(err)
	}
	newID, _, err := s.Fork("c1", m1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetRollingSummary(newID); ok {
		t.Fatal("forked conversation must not inherit rolling summary")
	}
	got, ok := s.GetGoalStack(newID)
	if !ok || len(got.Frames) != 1 {
		t.Fatalf("fork must copy goal stack: %+v ok=%v", got, ok)
	}
	if got.Frames[0].Text != "主目标" || got.Frames[0].Status != GoalActive || got.Frames[0].Revision != 3 {
		t.Fatalf("frame fields: %+v", got.Frames[0])
	}
	if got.Frames[0].ID == srcFrameID {
		t.Fatal("fork must assign new frame ids")
	}
	if got.Revision != 5 {
		t.Fatalf("revision=%d want 5", got.Revision)
	}
	src, _ := s.GetGoalStack("c1")
	if src.Frames[0].ID != srcFrameID {
		t.Fatal("source stack must be unchanged")
	}
}

func TestForkGoalCriteriaDeepCopied(t *testing.T) {
	s := NewMemoryStore()
	m1, err := s.Append("c1", Message{Role: RoleUser, Content: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c1", Message{Role: RoleUser, Content: "second"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertGoalStack(GoalStack{
		ConversationID: "c1",
		Frames: []GoalFrame{{
			ID: "g1", Text: "目标", Criteria: []string{"完成 A"}, Status: GoalActive, Source: GoalSourceUser,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	newID, _, err := s.Fork("c1", m1.ID)
	if err != nil {
		t.Fatal(err)
	}
	src, _ := s.GetGoalStack("c1")
	src.Frames[0].Criteria[0] = "mutated"
	forked, ok := s.GetGoalStack(newID)
	if !ok || len(forked.Frames[0].Criteria) != 1 || forked.Frames[0].Criteria[0] != "完成 A" {
		t.Fatalf("fork criteria must be independent: %+v", forked.Frames[0].Criteria)
	}
}
