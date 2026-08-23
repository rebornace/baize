package specstore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/connector/specstore"
)

func TestWriteStoresImportedAndNormalized(t *testing.T) {
	dir := t.TempDir()
	original := []byte(`{"swagger":"2.0"}`)
	normalized := []byte(`{"openapi":"3.0.3","paths":{"/x":{"get":{"responses":{"200":{"description":"ok"}}}}}}`)

	rel, err := specstore.Write(dir, "demo", original, normalized)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	wantRel := filepath.Join("connectors", "demo", "openapi.normalized.json")
	if rel != wantRel {
		t.Fatalf("rel path = %q, want %q", rel, wantRel)
	}

	imported := filepath.Join(dir, "connectors", "demo", "imported.bin")
	gotOriginal, err := os.ReadFile(imported)
	if err != nil {
		t.Fatalf("read imported: %v", err)
	}
	if string(gotOriginal) != string(original) {
		t.Fatalf("imported content mismatch")
	}

	normalizedPath := filepath.Join(dir, rel)
	gotNormalized, err := os.ReadFile(normalizedPath)
	if err != nil {
		t.Fatalf("read normalized: %v", err)
	}
	if string(gotNormalized) != string(normalized) {
		t.Fatalf("normalized content mismatch")
	}
}
