package conversation

import (
	"testing"
)

func TestMemoryContextProjectionRoundTripDoesNotTouchMessages(t *testing.T) {
	s := NewMemoryStore()
	m, err := s.Append("c1", Message{Role: RoleUser, Content: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertContextProjection(ContextProjection{
		ConversationID: "c1",
		Pins:           []string{"订单号 ORD-9"},
		Summary:        "用户在查订单",
		Dropped:        []string{"tc_1"},
		Revision:       2,
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := s.GetContextProjection("c1")
	if !ok || got.Summary != "用户在查订单" || len(got.Pins) != 1 || got.Pins[0] != "订单号 ORD-9" ||
		len(got.Dropped) != 1 || got.Dropped[0] != "tc_1" || got.Revision != 2 {
		t.Fatalf("round trip failed: %+v ok=%v", got, ok)
	}
	msgs := s.List("c1")
	if len(msgs) != 1 || msgs[0].ID != m.ID || msgs[0].Content != "hello" {
		t.Fatalf("messages must stay immutable: %+v", msgs)
	}
}

func TestMemoryTruncateClearsProjection(t *testing.T) {
	s := NewMemoryStore()
	m1, err := s.Append("c1", Message{Role: RoleUser, Content: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c1", Message{Role: RoleUser, Content: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertContextProjection(ContextProjection{ConversationID: "c1", Pins: []string{"p"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TruncateFrom("c1", m1.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetContextProjection("c1"); ok {
		t.Fatal("truncate must clear projection")
	}
}

func TestMemoryUpsertContextProjectionRequiresConversationID(t *testing.T) {
	s := NewMemoryStore()
	if err := s.UpsertContextProjection(ContextProjection{Pins: []string{"x"}}); err == nil {
		t.Fatal("expected error")
	}
}
