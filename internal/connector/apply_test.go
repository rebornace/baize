package connector_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func ptr[T any](v T) *T { return &v }

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

func writeLoginGetMeProbeSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "spec.yaml")
	content := `openapi: 3.0.3
info:
  title: login-getme-probe
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
  /probe:
    get:
      operationId: probe
      responses:
        "200":
          description: ok
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeAdminLoginSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "spec.yaml")
	content := `openapi: 3.0.3
info:
  title: admin-login
  version: 0.1.0
paths:
  /login:
    post:
      operationId: AdminAuthController_login
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
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestApplyPreservesRequireLoginWhenOmitted(t *testing.T) {
	spec := writeLoginGetMeSpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	base := connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "c", Type: "openapi", Spec: spec, BaseURL: "http://example.invalid",
		Auth: store.ConnectorAuth{Mode: "static"},
	}
	in1 := base
	in1.RequireLogin = ptr([]string{"getMe"})
	if _, _, err := connector.Apply(in1); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if !reg.RequiresLogin("getMe") {
		t.Fatal("getMe should require login after first Apply")
	}

	in2 := base
	in2.RequireLogin = nil
	if _, _, err := connector.Apply(in2); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if !reg.RequiresLogin("getMe") {
		t.Fatal("getMe should still require login when RequireLogin omitted")
	}

	probeSpec := writeLoginGetMeProbeSpec(t)
	in3 := base
	in3.Spec = probeSpec
	in3.RequireLogin = nil
	if _, _, err := connector.Apply(in3); err != nil {
		t.Fatalf("third Apply: %v", err)
	}
	if !reg.RequiresLogin("getMe") {
		t.Fatal("getMe should still require login after probe spec")
	}
	if reg.RequiresLogin("probe") {
		t.Fatal("probe must not require login")
	}
}

func TestApplyOmitsRequireLoginDropsDisappearedFromStore(t *testing.T) {
	probeSpec := writeLoginGetMeProbeSpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	base := connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "c", Type: "openapi", Spec: probeSpec, BaseURL: "http://example.invalid",
		Auth: store.ConnectorAuth{Mode: "static"},
	}
	in1 := base
	in1.RequireLogin = ptr([]string{"getMe", "probe"})
	if _, _, err := connector.Apply(in1); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	c1, err := st.GetConnector("c")
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(c1.RequireLogin, "getMe", "probe") {
		t.Fatalf("store require_login after first Apply: %v", c1.RequireLogin)
	}

	// Drop probe from the spec; omit require_login so surviving flags are preserved.
	in2 := base
	in2.Spec = writeLoginGetMeSpec(t)
	in2.RequireLogin = nil
	if _, _, err := connector.Apply(in2); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if !reg.RequiresLogin("getMe") {
		t.Fatal("getMe should still require login")
	}
	c2, err := st.GetConnector("c")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range c2.RequireLogin {
		if n == "probe" {
			t.Fatalf("store require_login must drop disappeared tool, got %v", c2.RequireLogin)
		}
	}
	if !containsAll(c2.RequireLogin, "getMe") {
		t.Fatalf("store require_login must keep surviving tool, got %v", c2.RequireLogin)
	}
}

func containsAll(list []string, names ...string) bool {
	set := map[string]bool{}
	for _, n := range list {
		set[n] = true
	}
	for _, n := range names {
		if !set[n] {
			return false
		}
	}
	return true
}

func TestApplyEmptyRequireLoginClears(t *testing.T) {
	spec := writeLoginGetMeSpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	base := connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "c", Type: "openapi", Spec: spec, BaseURL: "http://example.invalid",
		Auth: store.ConnectorAuth{Mode: "static"},
	}
	in1 := base
	in1.RequireLogin = ptr([]string{"getMe"})
	if _, _, err := connector.Apply(in1); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	in2 := base
	in2.RequireLogin = ptr([]string{})
	if _, _, err := connector.Apply(in2); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if reg.RequiresLogin("getMe") {
		t.Fatal("empty RequireLogin must clear getMe")
	}
}

func TestApplyOmitsCaptureUsesLoginGlob(t *testing.T) {
	const jwt = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhZG1pbkB4LmNvbSIsImVtYWlsIjoiYWRtaW5AeC5jb20iLCJyb2xlcyI6WyJhZG1pbiJdLCJleHAiOjk5OTk5OTk5OTl9.sig"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/login" {
			_, _ = w.Write([]byte(`{"accessToken":"` + jwt + `","email":"admin@x.com"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	spec := writeAdminLoginSpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	if _, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "c", Type: "openapi", Spec: spec, BaseURL: srv.URL,
		RequireLogin: ptr([]string{}),
		Auth:         store.ConnectorAuth{Mode: "static"},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	ctx := identity.WithConversationID(context.Background(), "conv_cap")
	_, isErr, err := reg.Invoke(ctx, "AdminAuthController_login", map[string]any{
		"email": "admin@x.com", "password": "x",
	})
	if err != nil || isErr {
		t.Fatalf("login invoke: isErr=%v err=%v", isErr, err)
	}
	if len(ids.List("conv_cap")) == 0 {
		t.Fatal("expected captured identity after login")
	}
}

func TestApplyHTTPIgnoresCapture(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"login","description":"login"}]}`))
		case strings.HasSuffix(r.URL.Path, "/invoke"):
			_, _ = w.Write([]byte(`{"content":{"accessToken":"tok","email":"a@x.com"},"is_error":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	if _, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "side", Type: "http", BaseURL: srv.URL,
		RequireLogin: ptr([]string{}),
		Auth: store.ConnectorAuth{
			Mode: "static",
			Capture: store.CaptureAuth{
				ToolNameGlob:   "*login*",
				TokenJSONPaths: []string{"accessToken"},
				LabelJSONPaths: []string{"email"},
				HeaderTemplate: "Bearer {{token}}",
			},
		},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	c, err := st.GetConnector("side")
	if err != nil {
		t.Fatal(err)
	}
	if c.Auth.Capture.ToolNameGlob != "" || len(c.Auth.Capture.TokenJSONPaths) != 0 {
		t.Fatalf("HTTP connector must clear Capture, got %+v", c.Auth.Capture)
	}

	ctx := identity.WithConversationID(context.Background(), "conv_http")
	_, isErr, invErr := reg.Invoke(ctx, "login", map[string]any{})
	if invErr != nil || isErr {
		t.Fatalf("invoke login: isErr=%v err=%v", isErr, invErr)
	}
	if len(ids.List("conv_http")) != 0 {
		t.Fatalf("HTTP plugin must not capture identities, got %+v", ids.List("conv_http"))
	}
}

func TestApplyEmptyStaticHeadersOK(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.yaml")
	content := "openapi: 3.0.3\ninfo:\n  title: bs\n  version: 0.1.0\npaths:\n  /probe:\n    get:\n      operationId: probe\n      responses:\n        \"200\":\n          description: ok\n"
	if err := os.WriteFile(spec, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	st := store.NewMemory()
	reg := tool.NewRegistry()
	login := []string(nil)
	_, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: identity.NewMemoryStore(),
		ID: "bs", Type: "openapi", Spec: spec, BaseURL: "http://example.invalid",
		RequireLogin: &login,
		Auth:         store.ConnectorAuth{Mode: "static"},
	})
	if err != nil {
		t.Fatal(err)
	}
}
