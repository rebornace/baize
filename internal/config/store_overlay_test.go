package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteStoreOverlay(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "minimal.yaml")
	if err := os.WriteFile(base, []byte("listen: :8080\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteStoreOverlay(base, StoreOverlay{
		Driver: "postgres",
		DSN:    "postgres://u:p@localhost/baize",
	}); err != nil {
		t.Fatal(err)
	}
	overlay := OverlayWritePath(base)
	b, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "postgres") {
		t.Fatalf("overlay=%q", string(b))
	}
}

func TestRedactDSN(t *testing.T) {
	in := "postgres://user:secret@localhost:5432/baize?sslmode=disable"
	out := RedactDSN(in)
	if strings.Contains(out, "secret") {
		t.Fatalf("redacted=%q", out)
	}
	if !strings.Contains(out, "***") {
		t.Fatalf("redacted=%q", out)
	}
}
