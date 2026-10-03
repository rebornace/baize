package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBAIZEListenOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(path, []byte("listen: \":8080\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BAIZE_LISTEN", "127.0.0.1:19090")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:19090" {
		t.Fatalf("Listen=%q", cfg.Listen)
	}
}

func TestBAIZEListenEmptyKeepsYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(path, []byte("listen: \":9090\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BAIZE_LISTEN", "  ")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":9090" {
		t.Fatalf("Listen=%q", cfg.Listen)
	}
}
