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

func TestUpdateMetaWorkspace(t *testing.T) {
	mem := conversation.NewMemoryStore()
	if err := mem.EnsureMeta(conversation.Meta{ID: "c-mem", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	if err := mem.UpdateMetaWorkspace("c-mem", "ws_lab"); err != nil {
		t.Fatal(err)
	}
	got, err := mem.GetMeta("c-mem")
	if err != nil || got.WorkspaceID != "ws_lab" {
		t.Fatalf("memory meta=%+v err=%v", got, err)
	}
	if err := mem.UpdateMetaWorkspace("missing", "ws_lab"); !errors.Is(err, conversation.ErrMetaNotFound) {
		t.Fatalf("memory missing err=%v", err)
	}

	db, _ := openTestDB(t)
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureMeta(conversation.Meta{ID: "c-sql", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateMetaWorkspace("c-sql", "ws_lab"); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetMeta("c-sql")
	if err != nil || got.WorkspaceID != "ws_lab" {
		t.Fatalf("sqlite meta=%+v err=%v", got, err)
	}
	if err := s.UpdateMetaWorkspace("missing", "ws_lab"); !errors.Is(err, conversation.ErrMetaNotFound) {
		t.Fatalf("sqlite missing err=%v", err)
	}
}

func TestUpdateMetaTitle(t *testing.T) {
	mem := conversation.NewMemoryStore()
	if err := mem.EnsureMeta(conversation.Meta{ID: "c-mem", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	if err := mem.UpdateMetaTitle("c-mem", "新标题"); err != nil {
		t.Fatal(err)
	}
	got, err := mem.GetMeta("c-mem")
	if err != nil || got.Title != "新标题" {
		t.Fatalf("memory title=%q err=%v", got.Title, err)
	}
	if err := mem.UpdateMetaTitle("c-mem", ""); err != nil {
		t.Fatal(err)
	}
	got, _ = mem.GetMeta("c-mem")
	if got.Title != "" {
		t.Fatalf("clear title=%q", got.Title)
	}
	if err := mem.UpdateMetaTitle("missing", "x"); !errors.Is(err, conversation.ErrMetaNotFound) {
		t.Fatalf("memory missing err=%v", err)
	}

	db, _ := openTestDB(t)
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureMeta(conversation.Meta{ID: "c-sql", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateMetaTitle("c-sql", "改名"); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetMeta("c-sql")
	if err != nil || got.Title != "改名" {
		t.Fatalf("sqlite title=%q err=%v", got.Title, err)
	}
	if err := s.UpdateMetaTitle("missing", "x"); !errors.Is(err, conversation.ErrMetaNotFound) {
		t.Fatalf("sqlite missing err=%v", err)
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

func TestMemoryDeleteMeta(t *testing.T) {
	s := conversation.NewMemoryStore()
	if err := s.EnsureMeta(conversation.Meta{ID: "c1", OwnerID: "alice", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	// Delete existing.
	if err := s.DeleteMeta("c1"); err != nil {
		t.Fatalf("DeleteMeta: %v", err)
	}
	if _, err := s.GetMeta("c1"); !errors.Is(err, conversation.ErrMetaNotFound) {
		t.Fatalf("after delete want ErrMetaNotFound got %v", err)
	}
	if list, _ := s.ListMeta(conversation.MetaFilter{}); len(list) != 0 {
		t.Fatalf("after delete list len=%d want 0", len(list))
	}
	// Deleting a missing id is idempotent.
	if err := s.DeleteMeta("c1"); err != nil {
		t.Fatalf("DeleteMeta missing should be no-op, got %v", err)
	}
}

func TestSQLiteDeleteMeta(t *testing.T) {
	db, _ := openTestDB(t)
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureMeta(conversation.Meta{ID: "c1", OwnerID: "alice", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteMeta("c1"); err != nil {
		t.Fatalf("DeleteMeta: %v", err)
	}
	if _, err := s.GetMeta("c1"); !errors.Is(err, conversation.ErrMetaNotFound) {
		t.Fatalf("after delete want ErrMetaNotFound got %v", err)
	}
	if list, _ := s.ListMeta(conversation.MetaFilter{}); len(list) != 0 {
		t.Fatalf("after delete list len=%d want 0", len(list))
	}
	if err := s.DeleteMeta("never-existed"); err != nil {
		t.Fatalf("DeleteMeta missing should be no-op, got %v", err)
	}
}

func TestMemoryUpdateMetaArchived(t *testing.T) {
	s := conversation.NewMemoryStore()
	if err := s.EnsureMeta(conversation.Meta{ID: "c1", OwnerID: "alice", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateMetaArchived("c1", true); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetMeta("c1")
	if err != nil || got.ArchivedAt.IsZero() {
		t.Fatalf("archived=%+v err=%v", got, err)
	}
	if err := s.UpdateMetaArchived("c1", false); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetMeta("c1")
	if !got.ArchivedAt.IsZero() {
		t.Fatalf("unarchive failed: %+v", got)
	}
	if err := s.UpdateMetaArchived("missing", true); !errors.Is(err, conversation.ErrMetaNotFound) {
		t.Fatalf("missing err=%v", err)
	}
}

func TestSQLiteUpdateMetaArchived(t *testing.T) {
	db, _ := openTestDB(t)
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureMeta(conversation.Meta{ID: "c1", OwnerID: "alice", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateMetaArchived("c1", true); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetMeta("c1")
	if err != nil || got.ArchivedAt.IsZero() {
		t.Fatalf("archived=%+v err=%v", got, err)
	}
	list, err := s.ListMeta(conversation.MetaFilter{})
	if err != nil || len(list) != 1 || list[0].ArchivedAt.IsZero() {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if err := s.UpdateMetaArchived("c1", false); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetMeta("c1")
	if !got.ArchivedAt.IsZero() {
		t.Fatalf("unarchive failed: %+v", got)
	}
}
