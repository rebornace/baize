package openapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

const registerCaptureJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhZG1pbkB4LmNvbSIsImVtYWlsIjoiYWRtaW5AeC5jb20iLCJyb2xlcyI6WyJhZG1pbiJdLCJleHAiOjk5OTk5OTk5OTl9.sig"

func TestRegisterWithOptsResolveAndCapture(t *testing.T) {
	var lastAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_, _ = w.Write([]byte(`{"accessToken":"` + registerCaptureJWT + `","email":"admin@x.com"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/me":
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	spec := writeLoginGetMeSpec(t)
	mem := identity.NewMemoryStore()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	envAuth := "Bearer ENV_TOKEN"
	_, _, err := openapi.RegisterWithOpts(st, reg, openapi.RegisterOpts{
		ID:       "auth-sample",
		Type:     "openapi",
		SpecPath: spec,
		BaseURL:  srv.URL,
		Headers:  map[string]string{"Authorization": envAuth},
		Identities: mem,
		Resolver:   authresolve.OpenAPISecurityResolver{},
		Capture: identity.CaptureConfig{
			ToolNameGlob:   "*login*",
			TokenJSONPaths: []string{"accessToken", "data.token"},
			LabelJSONPaths: []string{"email"},
			HeaderTemplate: "Bearer {{token}}",
			DefaultScheme:  "bearer",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1) No conversation ctx → getMe uses RegisterOpts.Headers (env).
	lastAuth = ""
	_, isErr, err := reg.Invoke(context.Background(), "getMe", nil)
	if err != nil || isErr {
		t.Fatalf("getMe without conv: isErr=%v err=%v", isErr, err)
	}
	if lastAuth != envAuth {
		t.Fatalf("no-conv Authorization=%q, want %q", lastAuth, envAuth)
	}

	// 2) With conversation: login captures token; subsequent getMe uses it.
	ctx := identity.WithConversationID(context.Background(), "conv_reg")
	_, isErr, err = reg.Invoke(ctx, "login", map[string]any{"email": "admin@x.com", "password": "x"})
	if err != nil || isErr {
		t.Fatalf("login: isErr=%v err=%v", isErr, err)
	}
	lastAuth = ""
	_, isErr, err = reg.Invoke(ctx, "getMe", nil)
	if err != nil || isErr {
		t.Fatalf("getMe after login: isErr=%v err=%v", isErr, err)
	}
	wantCaptured := "Bearer " + registerCaptureJWT
	if lastAuth != wantCaptured {
		t.Fatalf("after-login Authorization=%q, want %q", lastAuth, wantCaptured)
	}

	// 3) ListPublic has one entry and no plaintext token.
	views := mem.ListPublic("conv_reg")
	if len(views) != 1 {
		t.Fatalf("ListPublic=%+v", views)
	}
	raw, _ := json.Marshal(views)
	if strings.Contains(string(raw), registerCaptureJWT) || strings.Contains(string(raw), "ENV_TOKEN") {
		t.Fatalf("public list leaked token: %s", raw)
	}
}

func writeLoginGetMeSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "spec.yaml")
	content := `openapi: 3.0.3
info:
  title: login-getme
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
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                email: { type: string }
                password: { type: string }
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
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

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

func TestRegisterMutatingSkipsLoginCapture(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	spec := writeLoginGetMeSpec(t)
	_, _, err := openapi.RegisterWithOpts(st, reg, openapi.RegisterOpts{
		ID:                      "auth-sample",
		Type:                    "openapi",
		SpecPath:                spec,
		BaseURL:                 "http://example.invalid",
		RequireApprovalMutating: true,
		Capture: identity.CaptureConfig{
			ToolNameGlob: "*login*",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reg.RequiresApproval("login") {
		t.Fatal("login matched by capture glob must not require HITL under require_approval_mutating")
	}
	if reg.RequiresApproval("getMe") {
		t.Fatal("GET getMe must not require approval")
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
