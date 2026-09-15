package bootstrap

import (
	"context"
	"testing"

	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/config"
)

// TestEnsureBlobStoreDefaultsMemory: with no storage.driver (and no SQL),
// ensureBlobStore must still return a usable in-process Store.
func TestEnsureBlobStoreDefaultsMemory(t *testing.T) {
	cfg := config.Config{}
	cfg.Store.Driver = "memory"

	s, err := ensureBlobStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ensureBlobStore: %v", err)
	}
	if s == nil {
		t.Fatal("ensureBlobStore returned nil Store")
	}
	ctx := context.Background()
	if err := s.Put(ctx, "probe/key", []byte("ok"), "text/plain"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get(ctx, "probe/key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "ok" {
		t.Fatalf("body=%q want ok", got)
	}
}

// TestEnsureBlobStoreHonorsExplicitFile: yaml-configured file driver is kept.
func TestEnsureBlobStoreHonorsExplicitFile(t *testing.T) {
	cfg := config.Config{}
	cfg.Storage.Driver = "file"
	cfg.Storage.File.RootDir = t.TempDir()

	s, err := ensureBlobStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ensureBlobStore: %v", err)
	}
	if s == nil {
		t.Fatal("ensureBlobStore returned nil Store")
	}
	ctx := context.Background()
	if err := s.Put(ctx, "a/b", []byte("x"), ""); err != nil {
		t.Fatalf("Put: %v", err)
	}
}
