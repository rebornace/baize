package memory_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/memory"
	_ "modernc.org/sqlite"
)

func openSQLiteMemory(t *testing.T) memory.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	s, err := memory.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSQLiteUpsertSearchIsolationDelete(t *testing.T) {
	s := openSQLiteMemory(t)
	e, err := s.Upsert(memory.Entry{OwnerID: "alice", Text: "喜欢绿茶", Source: memory.SourceExplicit})
	if err != nil || e.ID == "" {
		t.Fatal(err, e)
	}
	_, _ = s.Upsert(memory.Entry{OwnerID: "bob", Text: "喜欢绿茶", Source: memory.SourceExplicit})

	got, err := s.Search("alice", "绿茶", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].OwnerID != "alice" {
		t.Fatalf("search: %+v", got)
	}

	if err := s.Delete("bob", e.ID); err == nil {
		t.Fatal("bob should not delete alice entry")
	}
	if err := s.Delete("alice", e.ID); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.Search("alice", "绿茶", 8)
	if len(got2) != 0 {
		t.Fatalf("after delete: %+v", got2)
	}
}
