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

// writeLoginGetMeProbeMutatingSpec writes a spec with a login (POST, capture
// glob target), a GET getMe, and a POST create_ticket (mutating, not a capture
// target) so RequireApprovalMutating can be exercised.
func writeLoginGetMeProbeMutatingSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "spec.yaml")
	content := `openapi: 3.0.3
info:
  title: login-getme-mutating
  version: 0.1.0
components:
  securitySchemes:
    bearer:
      type: http
      scheme: bearer
paths:
  /login:
    post:
      operationId: login
      security: []
      responses:
        "200":
          description: ok
  /me:
    get:
      operationId: getMe
      security:
        - bearer: []
      responses:
        "200":
          description: ok
  /tickets:
    post:
      operationId: create_ticket
      responses:
        "201":
          description: created`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestApplyRequireApprovalMutatingBakesIntoRow: when RequireApprovalMutating
// is true, Apply must bake RequireApproval=true into mutating spec rows so
// that listsFromTools, the persisted catalog, and RegisterOneFromConnector
// all observe the same flag. Login/capture tools (matching the capture glob
// with no explicit require_approval) are exempt.
func TestApplyRequireApprovalMutatingBakesIntoRow(t *testing.T) {
	spec := writeLoginGetMeProbeMutatingSpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	if _, _, err := connector.Apply(connector.ApplyInput{
		Store:      st,
		Registry:   reg,
		Identities: ids,
		ID:         "c",
		Type:       "openapi",
		Spec:       spec,
		BaseURL:    "http://example.invalid",
		Auth: store.ConnectorAuth{
			Mode: "static",
			Capture: store.CaptureAuth{
				ToolNameGlob: "*login*",
			},
		},
		RequireApprovalMutating: true,
		RequireLogin:            ptr([]string{}),
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// create_ticket is a mutating spec row → RequireApproval baked in.
	ct, err := st.GetTool("create_ticket")
	if err != nil {
		t.Fatalf("GetTool create_ticket: %v", err)
	}
	if !ct.RequireApproval {
		t.Fatalf("create_ticket.RequireApproval must be true after Apply with RequireApprovalMutating: %+v", ct)
	}
	if !reg.RequiresApproval("create_ticket") {
		t.Fatal("Registry.RequiresApproval(create_ticket) must be true")
	}

	// login is a mutating spec row but matches the capture glob and has no
	// explicit require_approval → exempt from blanket HITL.
	ln, err := st.GetTool("login")
	if err != nil {
		t.Fatalf("GetTool login: %v", err)
	}
	if ln.RequireApproval {
		t.Fatalf("login.RequireApproval must remain false (capture glob exemption): %+v", ln)
	}
	if reg.RequiresApproval("login") {
		t.Fatal("Registry.RequiresApproval(login) must be false (capture glob exemption)")
	}

	// getMe is GET → not mutating → no blanket HITL.
	gm, err := st.GetTool("getMe")
	if err != nil {
		t.Fatalf("GetTool getMe: %v", err)
	}
	if gm.RequireApproval {
		t.Fatalf("getMe.RequireApproval must remain false (GET): %+v", gm)
	}

	// RegisterOneFromConnector (PATCH path) must observe the baked-in flag:
	// unregister create_ticket, then re-register via the wrapper and confirm
	// the Registry still requires approval.
	c, err := st.GetConnector("c")
	if err != nil {
		t.Fatal(err)
	}
	reg.Unregister("create_ticket")
	if err := connector.RegisterOneFromConnector(st, reg, ids, c, ct, connector.CallbackConfig{}); err != nil {
		t.Fatalf("RegisterOneFromConnector: %v", err)
	}
	if !reg.RequiresApproval("create_ticket") {
		t.Fatal("RegisterOneFromConnector must preserve RequireApproval=true for create_ticket")
	}

	// The connector's aggregated require_approval list must include
	// create_ticket (listsFromTools sees the baked-in flag).
	if !containsAll(c.RequireApproval, "create_ticket") {
		t.Fatalf("connector.RequireApproval must include create_ticket: %v", c.RequireApproval)
	}
	for _, n := range c.RequireApproval {
		if n == "login" {
			t.Fatalf("login must not be in require_approval list: %v", c.RequireApproval)
		}
	}
}
