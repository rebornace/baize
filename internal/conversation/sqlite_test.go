package conversation_test

import (
	"database/sql"
	"path/filepath"
	"strings"
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

func TestSQLiteListSummariesTitleTruncateAndClear(t *testing.T) {
	db, _ := openTestDB(t)
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}

	long := strings.Repeat("啊", 45)
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: long}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleAssistant, Content: "ok"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c2", conversation.Message{Role: conversation.RoleUser, Content: "短标题"}); err != nil {
		t.Fatal(err)
	}
	sum := s.ListSummaries()
	if len(sum) != 2 {
		t.Fatalf("len=%d", len(sum))
	}
	if sum[0].ID != "c2" || sum[0].Title != "短标题" {
		t.Fatalf("newest=%+v", sum[0])
	}
	r := []rune(sum[1].Title)
	if sum[1].ID != "c1" || len(r) != 41 || !strings.HasSuffix(sum[1].Title, "…") {
		t.Fatalf("truncate=%q len=%d", sum[1].Title, len(r))
	}
	s.Clear("c2")
	sum = s.ListSummaries()
	if len(sum) != 1 || sum[0].ID != "c1" {
		t.Fatalf("after clear %+v", sum)
	}
}

func TestSQLiteListSummariesDefaultTitleWithoutUser(t *testing.T) {
	db, _ := openTestDB(t)
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleAssistant, Content: "仅助手"}); err != nil {
		t.Fatal(err)
	}
	sum := s.ListSummaries()
	if len(sum) != 1 || sum[0].ID != "c1" || sum[0].Title != "新对话" {
		t.Fatalf("want 新对话, got %+v", sum)
	}
}
