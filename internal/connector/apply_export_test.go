package connector_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func writeOrdersApproveSpec(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", "baize-spec-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	p := filepath.Join(dir, "spec.yaml")
	content := `openapi: 3.0.3
info:
  title: orders-approve
  version: 0.1.0
paths:
  /orders/approve:
    get:
      operationId: approveOrder
      responses:
        "200":
          description: ok
  /orders/{id}:
    get:
      operationId: getOrder
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestApplyOpenAPIVerbPathGetsForceDenyExport(t *testing.T) {
	spec := writeOrdersApproveSpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	_, _, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "c", Type: "openapi", Spec: spec, BaseURL: "http://example.invalid",
		Auth: store.ConnectorAuth{Mode: "static"},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	byName := map[string]store.Tool{}
	for _, tl := range st.ListTools() {
		if tl.ConnectorID == "c" {
			byName[tl.Name] = tl
		}
	}
	approve, ok := byName["approveOrder"]
	if !ok {
		t.Fatalf("missing approveOrder tool: %#v", byName)
	}
	if approve.Export != "force_deny" {
		t.Fatalf("approveOrder Export=%q want force_deny", approve.Export)
	}
	getOrder := byName["getOrder"]
	if getOrder.Export != "" {
		t.Fatalf("getOrder Export=%q want empty default", getOrder.Export)
	}
}
