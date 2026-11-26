package conversation_test

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
)

func messageRetentionCase(t *testing.T, st conversation.Store) {
	t.Helper()
	now := time.Now().UTC()
	conv := "retain-" + now.Format("150405.000000000")
	old := now.AddDate(0, 0, -40)
	ids := make([]string, 0, 5)
	for i := 0; i < 4; i++ {
		m, err := st.Append(conv, conversation.Message{
			Role: conversation.RoleUser, Content: "old", CreatedAt: old.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
	}
	recent, err := st.Append(conv, conversation.Message{
		Role: conversation.RoleAssistant, Content: "recent", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	cutoff := now.AddDate(0, 0, -30)
	if c, err := st.CountMessagesBefore(conv, cutoff, 2); err != nil || c != 3 {
		t.Fatalf("count=%d err=%v", c, err)
	}
	n, err := st.DeleteMessagesBefore(conv, cutoff, 2)
	if err != nil || n != 3 {
		t.Fatalf("deleted=%d err=%v", n, err)
	}
	left := st.List(conv)
	if len(left) != 2 || left[0].ID != ids[3] || left[1].ID != recent.ID {
		t.Fatalf("left=%+v", left)
	}
	// A conversation with fewer than `keep` messages is untouched.
	small := conv + "-small"
	if _, err := st.Append(small, conversation.Message{
		Role: conversation.RoleUser, Content: "only", CreatedAt: old,
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := st.DeleteMessagesBefore(small, now.AddDate(0, 0, -30), 2); err != nil || n != 0 {
		t.Fatalf("small deleted=%d err=%v", n, err)
	}
}

func TestMessageRetentionMemory(t *testing.T) {
	messageRetentionCase(t, conversation.NewMemoryStore())
}

func TestMessageRetentionSQLite(t *testing.T) {
	db, _ := openTestDB(t)
	st, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	messageRetentionCase(t, st)
}
