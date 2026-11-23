package specstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/connector/specstore"
)

func TestWriteStoresImportedAndNormalized(t *testing.T) {
	ctx := context.Background()
	s, err := blob.Open(ctx, "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"swagger":"2.0"}`)
	normalized := []byte(`{"openapi":"3.0.3","info":{"title":"t","version":"0.1.0"},"paths":{"/x":{"get":{"operationId":"ping","responses":{"200":{"description":"ok"}}}}}}`)

	key, err := specstore.Write(ctx, s, "demo", original, normalized)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	wantKey := blob.ConnectorNormalizedKey("demo")
	if key != wantKey {
		t.Fatalf("spec key = %q, want %q", key, wantKey)
	}
	if !specstore.IsBlobSpecKey(key) {
		t.Fatalf("IsBlobSpecKey(%q) = false", key)
	}

	gotOriginal, err := s.Get(ctx, blob.ConnectorImportedKey("demo"))
	if err != nil {
		t.Fatalf("get imported: %v", err)
	}
	if string(gotOriginal) != string(original) {
		t.Fatalf("imported content mismatch")
	}

	gotNormalized, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("get normalized: %v", err)
	}
	if string(gotNormalized) != string(normalized) {
		t.Fatalf("normalized content mismatch")
	}

	tools, err := openapi.LoadToolsFromBytes(gotNormalized)
	if err != nil {
		t.Fatalf("LoadToolsFromBytes: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected at least one tool from normalized blob")
	}

	if err := blob.DeletePrefix(ctx, s, "connectors/demo/"); err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	if _, err := s.Get(ctx, key); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("get after DeletePrefix: err=%v want ErrNotFound", err)
	}
}

func TestIsBlobSpecKey(t *testing.T) {
	if !specstore.IsBlobSpecKey("connectors/c1/openapi.normalized.json") {
		t.Fatal("expected blob key")
	}
	if specstore.IsBlobSpecKey(`/data/connectors/c1/openapi.normalized.json`) {
		t.Fatal("absolute path must not be a blob key")
	}
}
