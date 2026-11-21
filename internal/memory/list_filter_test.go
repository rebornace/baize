package memory_test

import (
	"testing"

	"github.com/rebornace/baize/internal/memory"
)

func mustUpsertFilter(t *testing.T, s memory.Store, e memory.Entry) memory.Entry {
	t.Helper()
	out, err := s.Upsert(e)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func ids(entries []memory.Entry) map[string]bool {
	out := make(map[string]bool, len(entries))
	for _, e := range entries {
		out[e.ID] = true
	}
	return out
}

func testListFiltered(t *testing.T, s memory.Store, owner, other string) {
	t.Helper()
	tea := mustUpsertFilter(t, s, memory.Entry{
		OwnerID: owner, Key: "Preference.Tea", Text: "Green tea", Source: memory.SourceExplicit,
	})
	_ = mustUpsertFilter(t, s, memory.Entry{
		OwnerID: owner, Key: "city", Text: "Hangzhou", Source: memory.SourceAuto,
	})
	black := mustUpsertFilter(t, s, memory.Entry{
		OwnerID: owner, Key: "Tea", Text: "Black tea", Source: memory.SourceAuto,
	})
	_ = mustUpsertFilter(t, s, memory.Entry{
		OwnerID: other, Key: "tea", Text: "Green tea", Source: memory.SourceExplicit,
	})
	pct := mustUpsertFilter(t, s, memory.Entry{
		OwnerID: owner, Key: "discount", Text: "100% tea", Source: memory.SourceExplicit,
	})

	all, err := s.ListFiltered(owner, "", "", 50, 0)
	if err != nil || len(all) != 4 {
		t.Fatalf("all=%d err=%v", len(all), err)
	}
	if _, ok := ids(all)[tea.ID]; !ok {
		t.Fatalf("owner filter lost tea: %+v", all)
	}

	got, err := s.ListFiltered(owner, "TEA", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !ids(got)[tea.ID] || !ids(got)[black.ID] || !ids(got)[pct.ID] {
		t.Fatalf("query tea=%+v", got)
	}

	got, err = s.ListFiltered(owner, "green", "", 50, 0)
	if err != nil || len(got) != 1 || got[0].ID != tea.ID {
		t.Fatalf("query green=%+v err=%v", got, err)
	}

	got, err = s.ListFiltered(owner, "", memory.SourceAuto, 50, 0)
	if err != nil || len(got) != 2 || !ids(got)[black.ID] {
		t.Fatalf("source auto=%+v err=%v", got, err)
	}

	got, err = s.ListFiltered(owner, "tea", memory.SourceAuto, 50, 0)
	if err != nil || len(got) != 1 || got[0].ID != black.ID {
		t.Fatalf("query+source=%+v err=%v", got, err)
	}
	got, err = s.ListFiltered(owner, "%", "", 50, 0)
	if err != nil || len(got) != 1 || got[0].ID != pct.ID {
		t.Fatalf("literal percent=%+v err=%v", got, err)
	}

	page, err := s.ListFiltered(owner, "", "", 2, 1)
	if err != nil || len(page) != 2 {
		t.Fatalf("page=%d err=%v", len(page), err)
	}
	if page[0].ID == page[1].ID {
		t.Fatalf("page duplicated entry: %+v", page)
	}
}

func TestListFiltered(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		testListFiltered(t, memory.NewMemoryStore(), "alice", "bob")
	})
	t.Run("sqlite", func(t *testing.T) {
		testListFiltered(t, openSQLiteMemory(t), "alice", "bob")
	})
	t.Run("postgres", func(t *testing.T) {
		s := openPostgresMemory(t)
		owner := uniqueOwner("filter-owner")
		other := uniqueOwner("filter-other")
		cleanupOwner(t, s, owner)
		cleanupOwner(t, s, other)
		testListFiltered(t, s, owner, other)
	})
}
