package conversation_test

import (
	"testing"

	"github.com/rebornace/baize/internal/conversation"
)

func TestMemoryStoreSearch(t *testing.T) {
	ms := conversation.NewMemoryStore()
	if _, err := ms.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "订单号 ORD-7788 已提交"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ms.Append("c1", conversation.Message{Role: conversation.RoleAssistant, Content: "收到"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ms.Append("c2", conversation.Message{Role: conversation.RoleUser, Content: "ORD-7788 别的会话"}); err != nil {
		t.Fatal(err)
	}
	hits, err := ms.Search("c1", "ORD-7788", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Content != "订单号 ORD-7788 已提交" {
		t.Fatalf("hits=%+v", hits)
	}
	empty, err := ms.Search("c1", "", 8)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty query: %+v err=%v", empty, err)
	}
}

func TestSQLiteStoreSearchFTS(t *testing.T) {
	db, _ := openTestDB(t)
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "绿茶偏好已记录"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleAssistant, Content: "好的"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "报销单 BX-42 驳回原因"}); err != nil {
		t.Fatal(err)
	}
	hits, err := s.Search("c1", "绿茶", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Content != "绿茶偏好已记录" {
		t.Fatalf("fts hits=%+v", hits)
	}
	hits2, err := s.Search("c1", "BX-42", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits2) != 1 {
		t.Fatalf("want BX-42 hit, got %+v", hits2)
	}
	// Compacted summary must not remove searchable originals.
	if err := s.UpsertRollingSummary(conversation.RollingSummary{
		ConversationID: "c1", Summary: "用户提过绿茶与报销",
		CoversThroughOrder: 1,
	}); err != nil {
		t.Fatal(err)
	}
	still, err := s.Search("c1", "绿茶", 8)
	if err != nil || len(still) != 1 {
		t.Fatalf("search after compact: %+v err=%v", still, err)
	}
}

func TestSQLiteStoreSearchBackfill(t *testing.T) {
	db, path := openTestDB(t)
	if _, err := db.Exec(`
CREATE TABLE messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  thinking TEXT,
  thinking_redacted INTEGER NOT NULL DEFAULT 0,
  run_id TEXT,
  created_at TEXT NOT NULL,
  pinned_skills TEXT NOT NULL DEFAULT '[]'
);
INSERT INTO messages (id, conversation_id, role, content, thinking_redacted, created_at, pinned_skills)
VALUES ('msg_1', 'c1', 'user', '遗留消息可检索', 0, '2020-01-01T00:00:00Z', '[]');
`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	db2 := reopenTestDB(t, path)
	s, err := conversation.OpenSQLite(db2)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := s.Search("c1", "遗留消息", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("backfill search: %+v", hits)
	}
}
