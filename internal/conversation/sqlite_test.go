package conversation_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "messages.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

func reopenTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSQLiteMessageRoundTrip(t *testing.T) {
	db, path := openTestDB(t)
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "ping"})
	if err != nil || got.ID == "" {
		t.Fatalf("id=%q err=%v", got.ID, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := conversation.OpenSQLite(reopenTestDB(t, path))
	if err != nil {
		t.Fatal(err)
	}
	list := s2.List("c1")
	if len(list) != 1 || list[0].Content != "ping" {
		t.Fatalf("%+v", list)
	}
}

func TestSQLiteAppendListClearWindow(t *testing.T) {
	db, _ := openTestDB(t)
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "你好"})
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

func TestSQLiteListWindowNonPositive(t *testing.T) {
	db, _ := openTestDB(t)
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "你好"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append("conv1", conversation.Message{Role: conversation.RoleAssistant, Content: "你好！"})
	if err != nil {
		t.Fatal(err)
	}
	all := s.List("conv1")

	for _, n := range []int{0, -1} {
		win := s.ListWindow("conv1", n)
		if len(win) != len(all) {
			t.Fatalf("n=%d: got %d messages, want %d", n, len(win), len(all))
		}
		if win[0].Role != conversation.RoleUser || win[1].Content != "你好！" {
			t.Fatalf("n=%d: window=%+v", n, win)
		}
	}
}
