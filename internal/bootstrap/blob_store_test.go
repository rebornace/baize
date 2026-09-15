package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/rebornace/baize/internal/blob/file"
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

// TestEnsureBlobStoreAfterLoadDefaultsMemory: config.Load must leave
// storage.driver empty, and ensureBlobStore then opens memory.
func TestEnsureBlobStoreAfterLoadDefaultsMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	body := "store:\n  driver: memory\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Storage.Driver != "" {
		t.Fatalf("Storage.Driver=%q want empty after Load", cfg.Storage.Driver)
	}
	s, err := ensureBlobStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ensureBlobStore: %v", err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "after-load/k", []byte("m"), ""); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := s.Get(ctx, "after-load/k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "m" {
		t.Fatalf("body=%q want m", got)
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

// TestEnsureBlobStoreAfterLoadExplicitFile: Load with storage.driver=file keeps file.
func TestEnsureBlobStoreAfterLoadExplicitFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "cfg.yaml")
	body := "store:\n  driver: memory\nstorage:\n  driver: file\n  file:\n    root_dir: " + filepath.ToSlash(root) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Storage.Driver != "file" {
		t.Fatalf("Storage.Driver=%q want file", cfg.Storage.Driver)
	}
	s, err := ensureBlobStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ensureBlobStore: %v", err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "explicit/f", []byte("y"), ""); err != nil {
		t.Fatalf("Put: %v", err)
	}
}
