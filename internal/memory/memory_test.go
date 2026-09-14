package memory_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/memory"
)

func TestMemoryUpsertListSearchForget(t *testing.T) {
	s := memory.NewMemoryStore()
	e, err := s.Upsert(memory.Entry{OwnerID: "alice", Text: "喜欢绿茶", Source: memory.SourceExplicit})
	if err != nil || e.ID == "" {
		t.Fatal(err, e)
	}
	_, _ = s.Upsert(memory.Entry{OwnerID: "bob", Text: "喜欢绿茶", Source: memory.SourceExplicit})
	got, _ := s.Search("alice", "绿茶", 8)
	if len(got) != 1 || got[0].OwnerID != "alice" {
		t.Fatalf("%+v", got)
	}
	n, _ := s.Forget("alice", "", "喜欢绿茶")
	if n != 1 {
		t.Fatalf("n=%d", n)
	}
}

func TestUpsertTruncatesMaxEntryChars(t *testing.T) {
	s := memory.NewMemoryStore()
	long := strings.Repeat("茶", memory.MaxEntryChars+1)
	e, err := s.Upsert(memory.Entry{OwnerID: "alice", Text: long, Source: memory.SourceExplicit})
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(e.Text)) != memory.MaxEntryChars {
		t.Fatalf("len=%d want %d", len([]rune(e.Text)), memory.MaxEntryChars)
	}
	if !strings.HasPrefix(e.Text, "茶") {
		t.Fatalf("text=%q", e.Text)
	}
}

func TestUpsertSameOwnerKeyOverwrites(t *testing.T) {
	s := memory.NewMemoryStore()
	first, err := s.Upsert(memory.Entry{OwnerID: "alice", Key: "pref", Text: "绿茶", Source: memory.SourceExplicit})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Upsert(memory.Entry{OwnerID: "alice", Key: "pref", Text: "红茶", Source: memory.SourceExplicit})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("id changed: %s -> %s", first.ID, second.ID)
	}
	if second.Text != "红茶" {
		t.Fatalf("text=%q", second.Text)
	}
	list, _ := s.List("alice", 0, 0)
	if len(list) != 1 {
		t.Fatalf("len=%d", len(list))
	}
	n, _ := s.Forget("alice", "pref", "")
	if n != 1 {
		t.Fatalf("forget n=%d", n)
	}
}

func TestUpsertByIDRebindsOwnerKeyIndex(t *testing.T) {
	s := memory.NewMemoryStore()
	e, err := s.Upsert(memory.Entry{OwnerID: "alice", Key: "a", Text: "v1", Source: memory.SourceExplicit})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upsert(memory.Entry{ID: e.ID, OwnerID: "alice", Key: "b", Text: "v2", Source: memory.SourceExplicit}); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Forget("alice", "a", ""); n != 0 {
		t.Fatalf("stale key a should be gone, n=%d", n)
	}
	if n, _ := s.Forget("alice", "b", ""); n != 1 {
		t.Fatalf("key b forget n=%d", n)
	}

	e2, err := s.Upsert(memory.Entry{OwnerID: "alice", Key: "c", Text: "v3", Source: memory.SourceExplicit})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upsert(memory.Entry{ID: e2.ID, OwnerID: "alice", Key: "", Text: "v4", Source: memory.SourceExplicit}); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Forget("alice", "c", ""); n != 0 {
		t.Fatalf("cleared key c should not forget, n=%d", n)
	}
	if n, _ := s.Forget("alice", "", "v4"); n != 1 {
		t.Fatalf("text forget n=%d", n)
	}
}
