package memory_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/rebornace/baize/internal/memory"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func openPostgresMemory(t *testing.T) memory.Store {
	t.Helper()
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err := memory.OpenPostgres(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPostgresUpsertSearchIsolationDelete(t *testing.T) {
	s := openPostgresMemory(t)
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

func TestPostgresUpsertSameOwnerKeyOverwrites(t *testing.T) {
	s := openPostgresMemory(t)
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
}

func TestOpenPostgresDriver(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	s, err := memory.Open("postgres", db)
	if err != nil {
		t.Fatal(err)
	}
	if s == nil {
		t.Fatal("nil store")
	}
}
