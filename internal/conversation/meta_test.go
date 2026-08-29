package conversation_test

import (
	"errors"
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

	got, err := s.GetMeta("c-alice")
	if err != nil || got.OwnerID != "alice" || got.Source != "ui" || got.Title != "A" {
		t.Fatalf("GetMeta=%+v err=%v", got, err)
	}

	// EnsureMeta is idempotent for existing id (keep original owner).
	if err := s.EnsureMeta(conversation.Meta{
		ID: "c-alice", OwnerID: "eve", Source: "weixin", UpdatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetMeta("c-alice")
	if err != nil || got.OwnerID != "alice" {
		t.Fatalf("owner overwritten: %+v err=%v", got, err)
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

func TestSQLiteGetMetaDistinguishesNotFoundAndParseError(t *testing.T) {
	db, _ := openTestDB(t)
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.GetMeta("missing")
	if !errors.Is(err, conversation.ErrMetaNotFound) {
		t.Fatalf("missing want ErrMetaNotFound got %v", err)
	}

	_, err = db.Exec(
		`INSERT INTO conversation_meta (id, owner_id, source, title, channel_peer, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"c-bad", "alice", "ui", nil, nil, "not-a-timestamp",
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.GetMeta("c-bad")
	if err == nil || errors.Is(err, conversation.ErrMetaNotFound) {
		t.Fatalf("parse/scan error must not look like not-found: %v", err)
	}
}

func TestMemoryEnsureGetListMeta(t *testing.T) {
	s := conversation.NewMemoryStore()
	now := time.Now().UTC()
	if err := s.EnsureMeta(conversation.Meta{ID: "c1", OwnerID: "alice", Source: "ui", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetMeta("c1")
	if err != nil || got.OwnerID != "alice" {
		t.Fatalf("%+v err=%v", got, err)
	}
	_, err = s.GetMeta("missing")
	if !errors.Is(err, conversation.ErrMetaNotFound) {
		t.Fatalf("want ErrMetaNotFound got %v", err)
	}
	list, err := s.ListMeta(conversation.MetaFilter{OwnerID: "alice"})
	if err != nil || len(list) != 1 {
		t.Fatalf("%+v err=%v", list, err)
	}
}
