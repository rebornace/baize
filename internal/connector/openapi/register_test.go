package openapi_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestRegisterConnectorConflictAndReplace(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	specA := filepath.Join("../../../examples/mock-ticket/openapi.yaml")

	_, infos, err := openapi.RegisterConnector(st, reg, "ticket-a", "openapi", specA, "http://a.example", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(infos, "create_ticket") {
		t.Fatalf("expected create_ticket after A: %+v", infos)
	}

	_, _, err = openapi.RegisterConnector(st, reg, "ticket-b", "openapi", specA, "http://b.example", nil)
	if err == nil {
		t.Fatal("expected conflict when B registers same tool names")
	}
	if !errors.Is(err, openapi.ErrToolConflict) && err.Error() != "tool_conflict" {
		t.Fatalf("expected tool_conflict, got %v", err)
	}

	// Conflict must leave connector A intact (no partial unregister).
	afterConflict := reg.List()
	if !hasTool(afterConflict, "create_ticket") || !hasTool(afterConflict, "list_tickets") {
		t.Fatalf("A tools missing after B conflict: %+v", afterConflict)
	}
	for _, info := range afterConflict {
		if info.ConnectorID != "ticket-a" {
			t.Fatalf("unexpected tool after B conflict: %+v", info)
		}
	}

	// Same id re-register with a different spec: old tools gone, new tools present.
	specB := writeMinimalSpec(t, "alt_op", "/alt")
	c, infos, err := openapi.RegisterConnector(st, reg, "ticket-a", "openapi", specB, "http://a2.example", []string{"alt_op"})
	if err != nil {
		t.Fatal(err)
	}
	if c.RequireApproval == nil || len(c.RequireApproval) != 1 || c.RequireApproval[0] != "alt_op" {
		t.Fatalf("RequireApproval=%v", c.RequireApproval)
	}
	if hasTool(infos, "create_ticket") {
		t.Fatalf("create_ticket should be gone after replace: %+v", infos)
	}
	if !hasTool(infos, "alt_op") {
		t.Fatalf("expected alt_op after replace: %+v", infos)
	}
	alt := findTool(infos, "alt_op")
	if alt == nil || !alt.RequireApproval {
		t.Fatalf("alt_op RequireApproval not wired in returned infos: %+v", infos)
	}
	listed := findTool(reg.List(), "alt_op")
	if listed == nil || !listed.RequireApproval || listed.ConnectorID != "ticket-a" {
		t.Fatalf("alt_op RequireApproval not wired in reg.List: %+v", reg.List())
	}
	got, err := st.GetConnector("ticket-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec != specB || got.BaseURL != "http://a2.example" {
		t.Fatalf("store connector=%+v", got)
	}
	if len(got.RequireApproval) != 1 || got.RequireApproval[0] != "alt_op" {
		t.Fatalf("store RequireApproval round-trip=%v", got.RequireApproval)
	}
}

func TestRegisterConnectorEmptyOpsInvalidSpec(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	specA := filepath.Join("../../../examples/mock-ticket/openapi.yaml")

	_, infos, err := openapi.RegisterConnector(st, reg, "ticket-a", "openapi", specA, "http://a.example", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTool(infos, "create_ticket") {
		t.Fatalf("expected create_ticket: %+v", infos)
	}

	emptySpec := writeEmptyPathsSpec(t)
	_, _, err = openapi.RegisterConnector(st, reg, "ticket-a", "openapi", emptySpec, "http://empty.example", nil)
	if err == nil {
		t.Fatal("expected invalid_spec for empty operations")
	}
	if !errors.Is(err, openapi.ErrInvalidSpec) {
		t.Fatalf("expected ErrInvalidSpec, got %v", err)
	}

	after := reg.List()
	if !hasTool(after, "create_ticket") || !hasTool(after, "list_tickets") {
		t.Fatalf("tools cleared after empty-ops put: %+v", after)
	}
	for _, info := range after {
		if info.ConnectorID != "ticket-a" {
			t.Fatalf("unexpected tool after empty-ops: %+v", info)
		}
	}
	got, err := st.GetConnector("ticket-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec != specA || got.BaseURL != "http://a.example" {
		t.Fatalf("store overwritten after empty-ops: %+v", got)
	}
}

func hasTool(infos []tool.Info, name string) bool {
	return findTool(infos, name) != nil
}

func findTool(infos []tool.Info, name string) *tool.Info {
	for i := range infos {
		if infos[i].Name == name {
			return &infos[i]
		}
	}
	return nil
}

func writeMinimalSpec(t *testing.T, opID, path string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "spec.yaml")
	content := "openapi: 3.0.3\n" +
		"info:\n  title: alt\n  version: 0.1.0\n" +
		"paths:\n  " + path + ":\n    get:\n      operationId: " + opID + "\n" +
		"      responses:\n        \"200\":\n          description: ok\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeEmptyPathsSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.yaml")
	content := "openapi: 3.0.3\n" +
		"info:\n  title: empty\n  version: 0.1.0\n" +
		"paths: {}\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
