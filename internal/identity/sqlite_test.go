package identity_test

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/identity"
	_ "modernc.org/sqlite"
)

func openSQL(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return db
}

func TestSQLiteStorePersistsAcrossOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id.db")
	db1 := openSQL(t, path)
	s1, err := identity.OpenSQLite(db1)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s1.Upsert("conv", identity.Identity{
		Label:             "a@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer SECRET"},
		Source:            identity.SourceLoginCapture,
		Subject:           "a@x.com",
		IsDefault:         true,
	})
	if err != nil || id == "" {
		t.Fatalf("upsert: id=%q err=%v", id, err)
	}
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	db2 := openSQL(t, path)
	s2, err := identity.OpenSQLite(db2)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	got, err := s2.Get("conv", id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CredentialHeaders["Authorization"] != "Bearer SECRET" {
		t.Fatalf("credential not persisted: %q", got.CredentialHeaders["Authorization"])
	}
	if !got.IsDefault {
		t.Fatal("is_default not persisted")
	}
}

func TestSQLiteStoreUpsertListDeleteDefault(t *testing.T) {
	s := newSQLiteStore(t)
	now := time.Now().UTC()
	id, err := s.Upsert("conv1", identity.Identity{
		Label:             "admin@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer SECRET_TOKEN"},
		Source:            identity.SourceLoginCapture,
		Subject:           "admin@x.com",
		IsDefault:         true,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || id == "" {
		t.Fatalf("upsert: id=%q err=%v", id, err)
	}
	views := s.ListPublic("conv1")
	if len(views) != 1 || views[0].Label != "admin@x.com" || !views[0].IsDefault {
		t.Fatalf("views=%+v", views)
	}
	raw, _ := json.Marshal(views)
	if strings.Contains(string(raw), "SECRET_TOKEN") {
		t.Fatal("public list leaked token")
	}
	if err := s.SetDefault("conv1", id); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("conv1", id); err != nil {
		t.Fatal(err)
	}
	if len(s.ListPublic("conv1")) != 0 {
		t.Fatal("expected empty")
	}
}

func TestSQLiteStoreUpsertSameSchemeSubject(t *testing.T) {
	s := newSQLiteStore(t)
	now := time.Now().UTC()
	id1, err := s.Upsert("conv1", identity.Identity{
		Label:             "admin@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer TOKEN1"},
		Source:            identity.SourceLoginCapture,
		Subject:           "admin@x.com",
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || id1 == "" {
		t.Fatalf("first upsert: id=%q err=%v", id1, err)
	}
	id2, err := s.Upsert("conv1", identity.Identity{
		Label:             "admin@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer TOKEN2"},
		Source:            identity.SourceLoginCapture,
		Subject:           "admin@x.com",
		UpdatedAt:         now,
	})
	if err != nil || id2 == "" {
		t.Fatalf("second upsert: id=%q err=%v", id2, err)
	}
	if id1 != id2 {
		t.Fatalf("expected same id, got %q and %q", id1, id2)
	}
	if len(s.ListPublic("conv1")) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(s.ListPublic("conv1")))
	}
	got, err := s.Get("conv1", id1)
	if err != nil {
		t.Fatal(err)
	}
	if got.CredentialHeaders["Authorization"] != "Bearer TOKEN2" {
		t.Fatalf("credential not updated: got %q", got.CredentialHeaders["Authorization"])
	}
}

func TestSQLiteStoreIsDefaultMutualExclusionOnUpsert(t *testing.T) {
	s := newSQLiteStore(t)
	now := time.Now().UTC()
	id1, err := s.Upsert("conv1", identity.Identity{
		Label:             "user1@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer T1"},
		Source:            identity.SourceLoginCapture,
		Subject:           "user1@x.com",
		IsDefault:         true,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || id1 == "" {
		t.Fatalf("first upsert: id=%q err=%v", id1, err)
	}
	id2, err := s.Upsert("conv1", identity.Identity{
		Label:             "user2@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer T2"},
		Source:            identity.SourceLoginCapture,
		Subject:           "user2@x.com",
		IsDefault:         true,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || id2 == "" {
		t.Fatalf("second upsert: id=%q err=%v", id2, err)
	}
	list := s.List("conv1")
	if countDefaults(list) != 1 {
		t.Fatalf("expected exactly 1 default, got %d in %+v", countDefaults(list), list)
	}
	got2, err := s.Get("conv1", id2)
	if err != nil {
		t.Fatal(err)
	}
	if !got2.IsDefault {
		t.Fatal("second identity should be default")
	}
	got1, err := s.Get("conv1", id1)
	if err != nil {
		t.Fatal(err)
	}
	if got1.IsDefault {
		t.Fatal("first identity should no longer be default")
	}
}

func TestSQLiteStoreIsDefaultMutualExclusionOnSetDefault(t *testing.T) {
	s := newSQLiteStore(t)
	now := time.Now().UTC()
	id1, err := s.Upsert("conv1", identity.Identity{
		Label:             "user1@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer T1"},
		Source:            identity.SourceLoginCapture,
		Subject:           "user1@x.com",
		IsDefault:         true,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || id1 == "" {
		t.Fatalf("first upsert: id=%q err=%v", id1, err)
	}
	id2, err := s.Upsert("conv1", identity.Identity{
		Label:             "user2@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer T2"},
		Source:            identity.SourceLoginCapture,
		Subject:           "user2@x.com",
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || id2 == "" {
		t.Fatalf("second upsert: id=%q err=%v", id2, err)
	}
	if err := s.SetDefault("conv1", id2); err != nil {
		t.Fatal(err)
	}
	list := s.List("conv1")
	if countDefaults(list) != 1 {
		t.Fatalf("expected exactly 1 default, got %d in %+v", countDefaults(list), list)
	}
	got2, err := s.Get("conv1", id2)
	if err != nil {
		t.Fatal(err)
	}
	if !got2.IsDefault {
		t.Fatal("second identity should be default after SetDefault")
	}
	got1, err := s.Get("conv1", id1)
	if err != nil {
		t.Fatal(err)
	}
	if got1.IsDefault {
		t.Fatal("first identity should no longer be default after SetDefault")
	}
}

func TestSQLiteStoreClearCaptured(t *testing.T) {
	s := newSQLiteStore(t)
	now := time.Now().UTC()
	if _, err := s.Upsert("conv1", identity.Identity{
		Label:         "env",
		Scheme:        "bearer",
		Source:        identity.SourceEnv,
		Subject:       "env",
		ClaimsSummary: map[string]any{"k": "v"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upsert("conv1", identity.Identity{
		Label:   "cap",
		Scheme:  "bearer",
		Source:  identity.SourceLoginCapture,
		Subject: "cap",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upsert("conv1", identity.Identity{
		Label:   "man",
		Scheme:  "bearer",
		Source:  identity.SourceManual,
		Subject: "man",
	}); err != nil {
		t.Fatal(err)
	}
	_ = now
	s.ClearCaptured("conv1")
	list := s.List("conv1")
	if len(list) != 1 || list[0].Source != identity.SourceEnv {
		t.Fatalf("after clear: %+v", list)
	}
}

func TestSQLiteStoreTouch(t *testing.T) {
	s := newSQLiteStore(t)
	id, err := s.Upsert("conv1", identity.Identity{
		Label:   "a",
		Scheme:  "bearer",
		Source:  identity.SourceEnv,
		Subject: "a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Touch("conv1", id); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("conv1", id)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastUsedAt.IsZero() {
		t.Fatal("last_used_at not set")
	}
	views := s.ListPublic("conv1")
	if len(views) != 1 || views[0].LastUsedAt == nil {
		t.Fatalf("public view missing last_used_at: %+v", views)
	}
}

func TestSQLiteStoreClaimsSummaryPreservedOnUpsertNil(t *testing.T) {
	s := newSQLiteStore(t)
	id, err := s.Upsert("conv1", identity.Identity{
		Label:         "a",
		Scheme:        "bearer",
		Source:        identity.SourceEnv,
		Subject:       "a",
		ClaimsSummary: map[string]any{"role": "admin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upsert("conv1", identity.Identity{
		Label:   "a-renamed",
		Scheme:  "bearer",
		Source:  identity.SourceEnv,
		Subject: "a",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("conv1", id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ClaimsSummary == nil || got.ClaimsSummary["role"] != "admin" {
		t.Fatalf("claims_summary should be preserved when upsert passes nil: %+v", got.ClaimsSummary)
	}
	if got.Label != "a-renamed" {
		t.Fatalf("label should be updated: %q", got.Label)
	}
}

func newSQLiteStore(t *testing.T) identity.Store {
	t.Helper()
	db := openSQL(t, filepath.Join(t.TempDir(), "id.db"))
	t.Cleanup(func() { _ = db.Close() })
	s, err := identity.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
