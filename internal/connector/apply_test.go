package connector_test

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

	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/connector/openapi"
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

func TestApplyHTTPPersistsAndCaptures(t *testing.T) {
	var lastAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[
				{"name":"login","description":"login"},
				{"name":"secure_ping","description":"needs auth"}
			]}`))
		case strings.HasSuffix(r.URL.Path, "/invoke"):
			if strings.Contains(r.URL.Path, "/tools/login/") {
				_, _ = w.Write([]byte(`{"content":{"accessToken":"tok-http","email":"a@x.com"},"is_error":false}`))
				return
			}
			lastAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
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
		RequireLogin: ptr([]string{"secure_ping"}),
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
	if c.Auth.Capture.ToolNameGlob != "*login*" || len(c.Auth.Capture.TokenJSONPaths) == 0 {
		t.Fatalf("HTTP connector must persist Capture, got %+v", c.Auth.Capture)
	}

	ctx := identity.WithConversationID(context.Background(), "conv_http")
	_, isErr, invErr := reg.Invoke(ctx, "login", map[string]any{})
	if invErr != nil || isErr {
		t.Fatalf("invoke login: isErr=%v err=%v", isErr, invErr)
	}
	if len(ids.List("conv_http")) == 0 {
		t.Fatal("expected captured identity after plugin login")
	}

	_, isErr, invErr = reg.Invoke(ctx, "secure_ping", map[string]any{})
	if invErr != nil || isErr {
		t.Fatalf("secure_ping: isErr=%v err=%v", isErr, invErr)
	}
	if lastAuth != "Bearer tok-http" {
		t.Fatalf("Authorization=%q want Bearer tok-http", lastAuth)
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

// TestApplyCatalogDisabledAndExtra: applying a spec, then disabling one row
// and adding an extra row via the Store, then re-applying with RequireLogin=nil
// must keep the disabled row out of the Registry, keep both rows in the Store,
// and register the extra row into the Registry.
func TestApplyCatalogDisabledAndExtra(t *testing.T) {
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
	in1.RequireLogin = ptr([]string{})
	if _, _, err := connector.Apply(in1); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if _, ok := reg.Get("login"); !ok {
		t.Fatal("login should be registered after first Apply")
	}
	if _, ok := reg.Get("getMe"); !ok {
		t.Fatal("getMe should be registered after first Apply")
	}

	// Disable getMe in the catalog and add an extra row.
	getMe, err := st.GetTool("getMe")
	if err != nil {
		t.Fatal(err)
	}
	getMe.Enabled = false
	st.UpsertTool(getMe)
	st.UpsertTool(store.Tool{
		ConnectorID: "c",
		Name:        "extraEcho",
		Source:      store.ToolSourceExtra,
		Enabled:     true,
		Method:      "GET",
		Path:        "/extra/echo",
		Description: "extra echo",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	})

	in2 := base
	in2.RequireLogin = nil
	if _, _, err := connector.Apply(in2); err != nil {
		t.Fatalf("second Apply: %v", err)
	}

	// Disabled name must not be in the Registry.
	if _, ok := reg.Get("getMe"); ok {
		t.Fatal("disabled getMe must not be registered")
	}
	// Disabled row must still be in the Store.
	got, err := st.GetTool("getMe")
	if err != nil {
		t.Fatalf("getMe store row missing: %v", err)
	}
	if got.Enabled {
		t.Fatalf("getMe must remain disabled in store: %+v", got)
	}
	// Extra row must be in the Store.
	if got, err := st.GetTool("extraEcho"); err != nil || got.Source != store.ToolSourceExtra {
		t.Fatalf("extraEcho missing in store: %+v err=%v", got, err)
	}
	// Extra row must be in the Registry.
	if _, ok := reg.Get("extraEcho"); !ok {
		t.Fatalf("extraEcho must be registered, registry=%+v", reg.List())
	}
	// login still registered.
	if _, ok := reg.Get("login"); !ok {
		t.Fatal("login must remain registered")
	}
}

// TestApplyBadSpecPreservesRowsAndRegistry: with pre-existing catalog rows and
// a registered connector, applying a bad spec must fail and leave both the
// Store catalog rows and the Registry exactly as they were before the failed
// Apply.
func TestApplyBadSpecPreservesRowsAndRegistry(t *testing.T) {
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

	// Snapshot state before the bad Apply.
	beforeRows := st.ListToolsByConnector("c")
	beforeInfos := reg.List()

	// Bad spec: empty paths → ErrInvalidSpec.
	dir := t.TempDir()
	badSpec := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(badSpec, []byte("openapi: 3.0.3\ninfo:\n  title: bad\n  version: 0.1.0\npaths: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in2 := base
	in2.Spec = badSpec
	_, _, err := connector.Apply(in2)
	if err == nil {
		t.Fatal("expected error from bad spec")
	}
	if !errors.Is(err, openapi.ErrInvalidSpec) {
		t.Fatalf("expected ErrInvalidSpec, got %v", err)
	}

	// Store catalog rows unchanged.
	afterRows := st.ListToolsByConnector("c")
	if len(afterRows) != len(beforeRows) {
		t.Fatalf("store rows changed: before=%+v after=%+v", beforeRows, afterRows)
	}
	beforeByName := map[string]store.Tool{}
	for _, r := range beforeRows {
		beforeByName[r.Name] = r
	}
	for _, r := range afterRows {
		b, ok := beforeByName[r.Name]
		if !ok {
			t.Fatalf("extra row after bad Apply: %+v", r)
		}
		if r.Enabled != b.Enabled || r.RequireLogin != b.RequireLogin {
			t.Fatalf("row changed: before=%+v after=%+v", b, r)
		}
	}

	// Registry unchanged.
	afterInfos := reg.List()
	if len(afterInfos) != len(beforeInfos) {
		t.Fatalf("registry size changed: before=%+v after=%+v", beforeInfos, afterInfos)
	}
	beforeReg := map[string]tool.Info{}
	for _, i := range beforeInfos {
		beforeReg[i.Name] = i
	}
	for _, i := range afterInfos {
		b, ok := beforeReg[i.Name]
		if !ok {
			t.Fatalf("extra registry entry after bad Apply: %+v", i)
		}
		if b.RequireLogin != i.RequireLogin || b.ConnectorID != i.ConnectorID {
			t.Fatalf("registry entry changed: before=%+v after=%+v", b, i)
		}
	}
}

// TestApplyPluginConnectorExtraRowNotRegistered: a plugin (type=http)
// connector's store must not have extra rows registered into the Registry.
// MergeCatalog preserves extras verbatim regardless of connector type, so
// without Apply's defense-in-depth guard an extra row on a plugin connector
// would route to openapiInvokerClosure (nil ctx.inv) and panic at invoke time.
// This test simulates a broken invariant (extra row mixed into a plugin
// connector's catalog) and asserts Apply skips it and Get does not panic.
func TestApplyPluginConnectorExtraRowNotRegistered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"echo"}]}`))
		case strings.HasSuffix(r.URL.Path, "/invoke"):
			_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	base := connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "side", Type: "http", BaseURL: srv.URL,
		Auth: store.ConnectorAuth{Mode: "static"},
	}
	login := []string{}
	in1 := base
	in1.RequireLogin = &login
	if _, _, err := connector.Apply(in1); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if _, ok := reg.Get("echo"); !ok {
		t.Fatal("echo should be registered after first Apply")
	}

	// Simulate a broken invariant: inject an extra row into the plugin
	// connector's catalog. MergeCatalog will preserve it verbatim.
	st.UpsertTool(store.Tool{
		ConnectorID: "side",
		Name:        "phantomExtra",
		Source:      store.ToolSourceExtra,
		Enabled:     true,
		Method:      "GET",
		Path:        "/phantom",
		Description: "phantom extra on a plugin connector",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	})

	in2 := base
	in2.RequireLogin = nil
	if _, _, err := connector.Apply(in2); err != nil {
		t.Fatalf("second Apply: %v", err)
	}

	// The extra row must not be registered (would nil-deref at invoke).
	if _, ok := reg.Get("phantomExtra"); ok {
		t.Fatalf("phantomExtra must not be registered on a plugin connector: %+v", reg.List())
	}
	// Get must not panic (defensive: confirm the name is absent).
	got, ok := reg.Get("phantomExtra")
	if ok {
		t.Fatalf("phantomExtra unexpectedly present: %+v", got)
	}
	// The plugin's own tool must still be registered.
	if _, ok := reg.Get("echo"); !ok {
		t.Fatal("echo must remain registered on the plugin connector")
	}
	// The extra row stays in the store (Apply does not delete it).
	if row, err := st.GetTool("phantomExtra"); err != nil || row.Source != store.ToolSourceExtra {
		t.Fatalf("phantomExtra should remain in store: %+v err=%v", row, err)
	}
}

// TestRegisterOneFromConnectorRegistersRow: the exported
// RegisterOneFromConnector wrapper must rebuild the registerOneContext from a
// persisted Connector + Tool row and register that single row. This proves the
// entry point exists for task 4 PATCH/POST handlers in internal/api.
func TestRegisterOneFromConnectorRegistersRow(t *testing.T) {
	spec := writeLoginGetMeSpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	if _, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "c", Type: "openapi", Spec: spec, BaseURL: "http://example.invalid",
		RequireLogin: ptr([]string{}),
		Auth:         store.ConnectorAuth{Mode: "static"},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	c, err := st.GetConnector("c")
	if err != nil {
		t.Fatal(err)
	}
	toolRow, err := st.GetTool("getMe")
	if err != nil {
		t.Fatal(err)
	}

	// Unregister getMe to prove the wrapper re-registers it.
	reg.Unregister("getMe")
	if _, ok := reg.Get("getMe"); ok {
		t.Fatal("getMe should be unregistered before wrapper call")
	}

	if err := connector.RegisterOneFromConnector(st, reg, ids, c, toolRow, connector.CallbackConfig{}); err != nil {
		t.Fatalf("RegisterOneFromConnector: %v", err)
	}
	info, ok := reg.Get("getMe")
	if !ok {
		t.Fatalf("getMe must be registered after RegisterOneFromConnector: %+v", reg.List())
	}
	if info.ConnectorID != "c" {
		t.Fatalf("ConnectorID=%q want c", info.ConnectorID)
	}
}

// TestRegisterOneFromConnectorRejectsPluginExtra: the exported wrapper must
// reject extra rows on plugin connectors with an error (matches Apply's
// defense-in-depth guard).
func TestRegisterOneFromConnectorRejectsPluginExtra(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"echo"}]}`))
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
		Auth:         store.ConnectorAuth{Mode: "static"},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	c, err := st.GetConnector("side")
	if err != nil {
		t.Fatal(err)
	}
	extra := store.Tool{
		ConnectorID: "side",
		Name:        "phantom",
		Source:      store.ToolSourceExtra,
		Enabled:     true,
		Method:      "GET",
		Path:        "/phantom",
		InputSchema: map[string]any{"type": "object"},
	}
	if err := connector.RegisterOneFromConnector(st, reg, ids, c, extra, connector.CallbackConfig{}); err == nil {
		t.Fatal("expected error registering extra on plugin connector, got nil")
	}
	if _, ok := reg.Get("phantom"); ok {
		t.Fatal("phantom must not be registered on a plugin connector")
	}
}

func writeEchoSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "spec.yaml")
	content := `openapi: 3.0.3
info:
  title: echo
  version: 0.1.0
paths:
  /echo:
    post:
      operationId: echo
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                msg: { type: string }
      responses:
        "200":
          description: ok
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestApplyOpenAPIEnterpriseCallback(t *testing.T) {
	var got struct {
		Tool           string         `json:"tool"`
		Arguments      map[string]any `json:"arguments"`
		RunID          string         `json:"run_id"`
		IdempotencyKey string         `json:"idempotency_key"`
	}
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode: %v", err)
		}
		_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
	}))
	t.Cleanup(callback.Close)

	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("direct HTTP should not be called when callback URL is set")
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	t.Cleanup(direct.Close)

	spec := writeEchoSpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	if _, _, err := connector.Apply(connector.ApplyInput{
		Store:                st,
		Registry:             reg,
		ID:                   "c",
		Type:                 "openapi",
		Spec:                 spec,
		BaseURL:              direct.URL,
		ExecutionCallbackURL: callback.URL + "/execute",
		Auth:                 store.ConnectorAuth{Mode: "static"},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	ctx := identity.WithRunID(context.Background(), "run_cb")
	ctx = identity.WithToolCallID(ctx, "call_xyz")
	_, isErr, err := reg.Invoke(ctx, "echo", map[string]any{"msg": "hi"})
	if err != nil || isErr {
		t.Fatalf("invoke: isErr=%v err=%v", isErr, err)
	}
	if got.Tool != "echo" || got.RunID != "run_cb" || got.IdempotencyKey != "call_xyz" {
		t.Fatalf("callback body=%+v", got)
	}
}
