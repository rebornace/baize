package memory_test

import (
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
