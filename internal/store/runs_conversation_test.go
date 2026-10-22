package store

import (
	"path/filepath"
	"testing"
	"time"
)

func runListRunsByConversationSinceCase(t *testing.T, st Store) {
	t.Helper()
	_, _ = st.CreateRun(CreateRunInput{AgentID: "a", Input: "early", ConversationID: "c1"})
	time.Sleep(2 * time.Millisecond)
	mid, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "mid", ConversationID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	_, _ = st.CreateRun(CreateRunInput{AgentID: "a", Input: "late", ConversationID: "c1"})
	_, _ = st.CreateRun(CreateRunInput{AgentID: "a", Input: "other", ConversationID: "c2"})

	got, err := st.ListRunsByConversationSince("c1", mid.CreatedAt, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 (mid+late), got %d", len(got))
	}
	if got[0].ID != mid.ID {
		t.Fatalf("order: want mid first, got %s", got[0].ID)
	}
}

func TestListRunsByConversationSinceMemory(t *testing.T) {
	runListRunsByConversationSinceCase(t, NewMemory())
}

func TestListRunsByConversationSinceSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs_conv.db")
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	runListRunsByConversationSinceCase(t, st)
}

func TestListRunsByConversationSinceEmptyConversationID(t *testing.T) {
	st := NewMemory()
	_, _ = st.CreateRun(CreateRunInput{AgentID: "a", Input: "x", ConversationID: "c1"})
	got, err := st.ListRunsByConversationSince("", time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil && len(got) != 0 {
		t.Fatalf("want empty slice, got %+v", got)
	}
}

func TestListRunsByConversationSinceDefaultLimit(t *testing.T) {
	st := NewMemory()
	for i := 0; i < 55; i++ {
		_, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "x", ConversationID: "c1"})
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := st.ListRunsByConversationSince("c1", time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 50 {
		t.Fatalf("limit<=0 should default to 50, got %d", len(got))
	}
}
