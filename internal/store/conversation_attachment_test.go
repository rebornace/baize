package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func conversationAttachmentCase(t *testing.T, st Store) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	conv := "weixin:acc:peer"
	older := ConversationAttachment{
		ConversationID: conv, Object: "a.png", Filename: "a.png", MediaType: "image/png",
		SizeBytes: 10, WorkspacePath: "uploads/a.png", CreatedAt: now.Add(-48 * time.Hour),
	}
	newer := ConversationAttachment{
		ConversationID: conv, Object: "b.pdf", Filename: "报告.pdf", MediaType: "application/pdf",
		SizeBytes: 20, WorkspacePath: "uploads/report.pdf", CreatedAt: now,
	}
	if err := st.AddConversationAttachment(older); err != nil {
		t.Fatal(err)
	}
	if err := st.AddConversationAttachment(newer); err != nil {
		t.Fatal(err)
	}
	got, ok, err := st.GetConversationAttachment(conv, "b.pdf")
	if err != nil || !ok || got.Filename != "报告.pdf" || got.WorkspacePath != "uploads/report.pdf" {
		t.Fatalf("get=%+v ok=%v err=%v", got, ok, err)
	}
	list, err := st.ListConversationAttachments(conv, 10)
	if err != nil || len(list) != 2 || list[0].Object != "b.pdf" || list[1].Object != "a.png" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if other, _ := st.ListConversationAttachments("other", 10); len(other) != 0 {
		t.Fatalf("other conversation leaked: %+v", other)
	}
	before, err := st.ListConversationAttachmentsBefore(now.Add(-time.Hour), 10)
	if err != nil || len(before) != 1 || before[0].Object != "a.png" {
		t.Fatalf("before=%+v err=%v", before, err)
	}
	if err := st.DeleteConversationAttachment(conv, "a.png"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.GetConversationAttachment(conv, "a.png"); ok {
		t.Fatal("attachment survived delete")
	}
	if err := st.DeleteConversationAttachment(conv, "a.png"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestConversationAttachmentMemory(t *testing.T) { conversationAttachmentCase(t, NewMemory()) }

func TestConversationAttachmentSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "attachments.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	conversationAttachmentCase(t, st)
}

func TestConversationAttachmentPostgres(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_ = st.DeleteConversationAttachment("weixin:acc:peer", "a.png")
	_ = st.DeleteConversationAttachment("weixin:acc:peer", "b.pdf")
	conversationAttachmentCase(t, st)
}
