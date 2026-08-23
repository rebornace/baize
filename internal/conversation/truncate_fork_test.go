package conversation_test

import (
	"errors"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
)

func TestTruncateFromAndFork(t *testing.T) {
	s := conversation.NewMemoryStore()
	_, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	m2, err := s.Append("c1", conversation.Message{Role: conversation.RoleAssistant, Content: "a1", RunID: "run_1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "u2"})
	if err != nil {
		t.Fatal(err)
	}

	deleted, err := s.TruncateFrom("c1", m2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Fatalf("deleted=%d want 2", deleted)
	}
	all := s.List("c1")
	if len(all) != 1 || all[0].Content != "u1" {
		t.Fatalf("after truncate: %+v", all)
	}

	_, err = s.Append("c1", conversation.Message{Role: conversation.RoleAssistant, Content: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "u2"})
	if err != nil {
		t.Fatal(err)
	}
	first := s.List("c1")[0]

	newID, copied, err := s.Fork("c1", first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 1 {
		t.Fatalf("copied=%d want 1", copied)
	}
	forked := s.List(newID)
	if len(forked) != 1 || forked[0].Content != "u1" || forked[0].ID == first.ID {
		t.Fatalf("forked: %+v", forked)
	}
	if len(s.List("c1")) != 3 {
		t.Fatalf("source should remain 3 messages")
	}
}

func TestTruncateFromNotFound(t *testing.T) {
	s := conversation.NewMemoryStore()
	_, err := s.TruncateFrom("c1", "msg_missing")
	if !errors.Is(err, conversation.ErrMessageNotFound) {
		t.Fatalf("err=%v", err)
	}
}
