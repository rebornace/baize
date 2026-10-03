package memory_test

import (
	"testing"

	"github.com/rebornace/baize/internal/memory"
)

func TestRankSubstringHit(t *testing.T) {
	if memory.Rank("绿茶", "喜欢绿茶") <= 0 {
		t.Fatal("expected substring hit")
	}
	if memory.Rank("咖啡", "喜欢绿茶") != 0 {
		t.Fatal("expected no match")
	}
	if memory.Rank("", "喜欢绿茶") != 0 {
		t.Fatal("empty query should not match")
	}
}

func TestSearchOrdersByRank(t *testing.T) {
	s := memory.NewMemoryStore()
	_, _ = s.Upsert(memory.Entry{OwnerID: "alice", Text: "偶尔喝绿茶", Source: memory.SourceExplicit})
	_, _ = s.Upsert(memory.Entry{OwnerID: "alice", Text: "最爱绿茶", Source: memory.SourceExplicit})
	got, err := s.Search("alice", "绿茶", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Text != "最爱绿茶" {
		t.Fatalf("want stronger match first, got %+v", got)
	}
}
