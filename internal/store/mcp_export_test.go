package store_test

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func testMCPExportIdentityKeyCRUD(t *testing.T, s store.Store) {
	t.Helper()

	now := time.Now().UTC().Truncate(time.Millisecond)
	identity := store.MCPExportIdentity{
		ID:      "idt_export_1",
		Name:    "Ops Bot",
		Scheme:  "Bearer",
		Headers: map[string]string{"Authorization": "Bearer secret-token"},
	}
	if err := s.UpsertMCPExportIdentity(identity); err != nil {
		t.Fatalf("UpsertMCPExportIdentity: %v", err)
	}

	got, err := s.GetMCPExportIdentity(identity.ID)
	if err != nil {
		t.Fatalf("GetMCPExportIdentity: %v", err)
	}
	if got.Name != identity.Name || got.Scheme != identity.Scheme {
		t.Fatalf("identity=%+v", got)
	}
	if got.Headers["Authorization"] != "Bearer secret-token" {
		t.Fatalf("headers=%v", got.Headers)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not set: %+v", got)
	}

	createdAt := got.CreatedAt
	identity.Name = "Ops Bot Updated"
	if err := s.UpsertMCPExportIdentity(identity); err != nil {
		t.Fatalf("UpsertMCPExportIdentity update: %v", err)
	}
	got, err = s.GetMCPExportIdentity(identity.ID)
	if err != nil || got.Name != "Ops Bot Updated" {
		t.Fatalf("updated identity=%+v err=%v", got, err)
	}
	if !got.CreatedAt.Equal(createdAt) || got.UpdatedAt.Before(createdAt) {
		t.Fatalf("updated timestamps=%+v created=%v", got, createdAt)
	}

	keyHash := "deadbeef" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	key := store.MCPExportKey{
		ID:         "key_1",
		Name:       "Primary",
		IdentityID: identity.ID,
		KeyHash:    keyHash,
		Prefix:     "bze_abcd",
		CreatedAt:  now,
	}
	if err := s.InsertMCPExportKey(key); err != nil {
		t.Fatalf("InsertMCPExportKey: %v", err)
	}

	keys, err := s.ListMCPExportKeys(identity.ID)
	if err != nil || len(keys) != 1 {
		t.Fatalf("ListMCPExportKeys=%+v err=%v", keys, err)
	}
	if keys[0].Prefix != "bze_abcd" || keys[0].KeyHash != keyHash {
		t.Fatalf("listed key=%+v", keys[0])
	}
	raw, err := json.Marshal(keys[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" {
		t.Fatal("empty json")
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["key_hash"]; ok {
		t.Fatalf("key_hash leaked in JSON: %s", raw)
	}

	if err := s.RevokeMCPExportKey(key.ID); err != nil {
		t.Fatalf("RevokeMCPExportKey: %v", err)
	}
	revoked, err := s.GetMCPExportKey(key.ID)
	if err != nil {
		t.Fatalf("GetMCPExportKey: %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatalf("expected RevokedAt, got %+v", revoked)
	}

	_, err = s.LookupMCPExportKeyByHash(keyHash)
	if err == nil {
		t.Fatal("expected lookup to skip revoked key")
	}

	identities, err := s.ListMCPExportIdentities()
	if err != nil || len(identities) != 1 {
		t.Fatalf("ListMCPExportIdentities=%+v err=%v", identities, err)
	}

	if err := s.DeleteMCPExportIdentity(identity.ID); err != nil {
		t.Fatalf("DeleteMCPExportIdentity: %v", err)
	}
	if _, err := s.GetMCPExportIdentity(identity.ID); err == nil {
		t.Fatal("expected identity deleted")
	}
	keys, err = s.ListMCPExportKeys(identity.ID)
	if err != nil || len(keys) != 0 {
		t.Fatalf("keys after delete=%+v err=%v", keys, err)
	}
}

func testMCPExportDuplicateKeyHash(t *testing.T, s store.Store) {
	t.Helper()

	identity := store.MCPExportIdentity{ID: "idt_dup", Name: "Dup Test"}
	if err := s.UpsertMCPExportIdentity(identity); err != nil {
		t.Fatal(err)
	}

	hash := "abc123" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab"
	key1 := store.MCPExportKey{
		ID: "key_a", Name: "A", IdentityID: identity.ID, KeyHash: hash, Prefix: "bze_aaaa",
	}
	if err := s.InsertMCPExportKey(key1); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	key2 := store.MCPExportKey{
		ID: "key_b", Name: "B", IdentityID: identity.ID, KeyHash: hash, Prefix: "bze_bbbb",
	}
	err := s.InsertMCPExportKey(key2)
	if !errors.Is(err, store.ErrMCPExportKeyHashExists) {
		t.Fatalf("second insert err=%v want ErrMCPExportKeyHashExists", err)
	}

	keys, err := s.ListMCPExportKeys(identity.ID)
	if err != nil || len(keys) != 1 {
		t.Fatalf("expected one key, got %+v err=%v", keys, err)
	}
}

func testMCPExportDeleteCascadesAllKeys(t *testing.T, s store.Store) {
	t.Helper()

	identity := store.MCPExportIdentity{ID: "idt_cascade", Name: "Cascade"}
	if err := s.UpsertMCPExportIdentity(identity); err != nil {
		t.Fatal(err)
	}

	for i, id := range []string{"key_c1", "key_c2"} {
		k := store.MCPExportKey{
			ID: id, Name: id, IdentityID: identity.ID,
			KeyHash: "hash_" + id + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			Prefix:  "bze_casc" + string(rune('0'+i)),
		}
		if err := s.InsertMCPExportKey(k); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	if err := s.DeleteMCPExportIdentity(identity.ID); err != nil {
		t.Fatalf("DeleteMCPExportIdentity: %v", err)
	}
	if _, err := s.GetMCPExportIdentity(identity.ID); err == nil {
		t.Fatal("identity should be gone")
	}
	keys, err := s.ListMCPExportKeys(identity.ID)
	if err != nil || len(keys) != 0 {
		t.Fatalf("all keys should be deleted atomically, got %+v err=%v", keys, err)
	}
}

func TestMemoryMCPExportIdentityKeyCRUD(t *testing.T) {
	testMCPExportIdentityKeyCRUD(t, store.NewMemory())
}

func TestMemoryMCPExportDuplicateKeyHash(t *testing.T) {
	testMCPExportDuplicateKeyHash(t, store.NewMemory())
}

func TestMemoryMCPExportDeleteCascadesAllKeys(t *testing.T) {
	testMCPExportDeleteCascadesAllKeys(t, store.NewMemory())
}

func TestSQLiteMCPExportIdentityKeyCRUD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp-export.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s.(io.Closer); ok {
			_ = c.Close()
		}
	})
	testMCPExportIdentityKeyCRUD(t, s)
}

func TestSQLiteMCPExportDuplicateKeyHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp-export-dup.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s.(io.Closer); ok {
			_ = c.Close()
		}
	})
	testMCPExportDuplicateKeyHash(t, s)
}

func TestSQLiteMCPExportDeleteCascadesAllKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp-export-cascade.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s.(io.Closer); ok {
			_ = c.Close()
		}
	})
	testMCPExportDeleteCascadesAllKeys(t, s)
}

func TestPostgresMCPExportIdentityKeyCRUD(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	s, err := store.OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	testMCPExportIdentityKeyCRUD(t, s)
}
