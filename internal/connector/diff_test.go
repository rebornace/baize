package connector_test

import (
	"testing"

	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/store"
)

func TestDiffCatalogAddedRemovedChanged(t *testing.T) {
	existing := []store.Tool{
		{Name: "get_order", Source: store.ToolSourceSpec, Method: "GET", Path: "/orders/{id}", OperationID: "getOrder", InputSchema: map[string]any{"type": "object"}},
		{Name: "gone", Source: store.ToolSourceSpec, Method: "GET", Path: "/gone"},
		{Name: "extra1", Source: store.ToolSourceExtra, Method: "GET", Path: "/x"},
	}
	merged := []store.Tool{
		{Name: "get_order", Source: store.ToolSourceSpec, Method: "GET", Path: "/v2/orders/{id}", OperationID: "getOrder", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}}},
		{Name: "approve", Source: store.ToolSourceSpec, Method: "GET", Path: "/orders/approve"},
		{Name: "extra1", Source: store.ToolSourceExtra, Method: "GET", Path: "/x"},
	}
	d := connector.DiffCatalog(existing, merged)
	if len(d.Added) != 1 || d.Added[0] != "approve" {
		t.Fatalf("added=%v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0] != "gone" {
		t.Fatalf("removed=%v (extra must not appear)", d.Removed)
	}
	if len(d.Changed) != 1 || d.Changed[0].Name != "get_order" {
		t.Fatalf("changed=%v", d.Changed)
	}
	if !d.Changed[0].MethodPathChanged || !d.Changed[0].SchemaChanged {
		t.Fatalf("change flags=%+v", d.Changed[0])
	}
}
