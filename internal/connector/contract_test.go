package connector_test

import (
	"testing"

	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/store"
)

func TestContractHashStableAndSensitive(t *testing.T) {
	base := store.Tool{
		Name:        "get_ticket",
		Method:      "GET",
		Path:        "/tickets/{id}",
		OperationID: "getTicket",
		Source:      store.ToolSourceSpec,
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}},
	}
	h1 := connector.ContractHash(base)
	h2 := connector.ContractHash(base)
	if h1 == "" || h1 != h2 {
		t.Fatalf("hash unstable: %q vs %q", h1, h2)
	}
	if len(h1) != 16 {
		t.Fatalf("len=%d want 16", len(h1))
	}
	changed := base
	changed.Path = "/v2/tickets/{id}"
	if connector.ContractHash(changed) == h1 {
		t.Fatal("path change should alter hash")
	}
	schema := base
	schema.InputSchema = map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "integer"}}}
	if connector.ContractHash(schema) == h1 {
		t.Fatal("schema change should alter hash")
	}
}

func TestCatalogDiffEmpty(t *testing.T) {
	var empty connector.CatalogDiff
	if !empty.Empty() {
		t.Fatal("empty diff should report Empty")
	}
	d := connector.CatalogDiff{Added: []string{"a"}}
	if d.Empty() {
		t.Fatal("non-empty Added should not be Empty")
	}
}
