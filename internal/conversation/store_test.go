package conversation_test

import (
	"testing"

	"github.com/rebornace/baize/internal/conversation"
)

func TestMemoryStoreAppendListClearWindow(t *testing.T) {
	s := conversation.NewMemoryStore()
	_, err := s.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "你好"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append("conv1", conversation.Message{Role: conversation.RoleAssistant, Content: "你好！", RunID: "run_1"})
	if err != nil {
		t.Fatal(err)
	}
	all := s.List("conv1")
	if len(all) != 2 || all[0].Role != conversation.RoleUser || all[1].Content != "你好！" {
		t.Fatalf("%+v", all)
	}
	win := s.ListWindow("conv1", 1)
	if len(win) != 1 || win[0].Role != conversation.RoleAssistant {
		t.Fatalf("window=%+v", win)
	}
	s.Clear("conv1")
	if len(s.List("conv1")) != 0 {
		t.Fatal("expected empty after clear")
	}
}
