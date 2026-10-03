package connector_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/connector/specstore"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestApplyLoadsToolsFromBlobSpecKey(t *testing.T) {
	ctx := context.Background()
	blobs, err := blob.Open(ctx, "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	normalized := []byte(`{"openapi":"3.0.3","info":{"title":"t","version":"0.1.0"},"paths":{"/x":{"get":{"operationId":"ping","responses":{"200":{"description":"ok"}}}}}}`)
	key, err := specstore.Write(ctx, blobs, "blob-c", []byte(`raw`), normalized)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	st := store.NewMemory()
	reg := tool.NewRegistry()
	login := []string{}
	_, infos, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: identity.NewMemoryStore(),
		Blobs: blobs,
		ID:    "blob-c", Type: "openapi", Spec: key, BaseURL: "http://example.invalid",
		RequireLogin: &login,
		Auth:         store.ConnectorAuth{Mode: "static"},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("expected tools from blob spec")
	}
	found := false
	for _, info := range infos {
		if info.Name == "ping" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ping tool, got %+v", infos)
	}

	if err := blob.DeletePrefix(ctx, blobs, "connectors/blob-c/"); err != nil {
		t.Fatal(err)
	}
	if _, err := blobs.Get(ctx, key); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
}

func TestApplyRejectsLegacyConnectorFSPath(t *testing.T) {
	legacy := filepath.Join(t.TempDir(), "connectors", "c1", "openapi.normalized.json")
	_, _, err := connector.Apply(connector.ApplyInput{
		Store: store.NewMemory(), Registry: tool.NewRegistry(), Identities: identity.NewMemoryStore(),
		ID: "c1", Type: "openapi", Spec: legacy, BaseURL: "http://example.invalid",
		Auth: store.ConnectorAuth{Mode: "static"},
	})
	if err == nil {
		t.Fatal("expected error for legacy filesystem spec path")
	}
	if !errors.Is(err, openapi.ErrInvalidSpec) {
		t.Fatalf("err=%v want ErrInvalidSpec", err)
	}
}
