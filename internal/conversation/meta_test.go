package conversation_test

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
)

func TestCanAccess(t *testing.T) {
	meta := conversation.Meta{ID: "c1", OwnerID: "alice", Source: "ui"}
	if !conversation.CanAccess(controlplane.Principal{Role: controlplane.RoleAdmin}, meta) {
		t.Fatal("admin must access any meta")
	}
	if !conversation.CanAccess(controlplane.Principal{Role: controlplane.RoleOperator, OperatorID: "alice"}, meta) {
		t.Fatal("owner must access")
	}
	if conversation.CanAccess(controlplane.Principal{Role: controlplane.RoleOperator, OperatorID: "bob"}, meta) {
		t.Fatal("non-owner must not access")
	}
}

func TestSQLiteEnsureGetListMeta(t *testing.T) {
	db, _ := openTestDB(t)
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := s.EnsureMeta(conversation.Meta{
		ID: "c-alice", OwnerID: "alice", Source: "ui", Title: "A", UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureMeta(conversation.Meta{
		ID: "c-bob", OwnerID: "bob", Source: "ui", UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	got, ok := s.GetMeta("c-alice")
	if !ok || got.OwnerID != "alice" || got.Source != "ui" || got.Title != "A" {
		t.Fatalf("GetMeta=%+v ok=%v", got, ok)
	}

	// EnsureMeta is idempotent for existing id (keep original owner).
	if err := s.EnsureMeta(conversation.Meta{
		ID: "c-alice", OwnerID: "eve", Source: "weixin", UpdatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	got, ok = s.GetMeta("c-alice")
	if !ok || got.OwnerID != "alice" {
		t.Fatalf("owner overwritten: %+v", got)
	}

	all, err := s.ListMeta(conversation.MetaFilter{})
	if err != nil || len(all) != 2 {
		t.Fatalf("all=%d err=%v", len(all), err)
	}
	mine, err := s.ListMeta(conversation.MetaFilter{OwnerID: "alice"})
	if err != nil || len(mine) != 1 || mine[0].ID != "c-alice" {
		t.Fatalf("mine=%+v err=%v", mine, err)
	}
}

func TestMemoryEnsureGetListMeta(t *testing.T) {
	s := conversation.NewMemoryStore()
	now := time.Now().UTC()
	if err := s.EnsureMeta(conversation.Meta{ID: "c1", OwnerID: "alice", Source: "ui", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	got, ok := s.GetMeta("c1")
	if !ok || got.OwnerID != "alice" {
		t.Fatalf("%+v ok=%v", got, ok)
	}
	list, err := s.ListMeta(conversation.MetaFilter{OwnerID: "alice"})
	if err != nil || len(list) != 1 {
		t.Fatalf("%+v err=%v", list, err)
	}
}
