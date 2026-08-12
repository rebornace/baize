package openapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	mockticket "github.com/rebornace/baize/examples/mock-ticket"
	"github.com/rebornace/baize/internal/connector/openapi"
)

func TestLoadToolsSecurityFromGlobal(t *testing.T) {
	spec := `openapi: "3.0.3"
info:
  title: security fixture
  version: "1.0.0"
components:
  securitySchemes:
    bearer:
      type: http
      scheme: bearer
security:
  - bearer: []
paths:
  /me:
    get:
      operationId: getMe
      responses:
        "200": { description: ok }
`
	path := writeTempSpec(t, spec)
	tools, err := openapi.LoadTools(path)
	if err != nil {
		t.Fatal(err)
	}
	route := findRoute(t, tools, "getMe")
	if len(route.Security) != 1 || route.Security[0] != "bearer" {
		t.Fatalf("Security=%v, want [bearer]", route.Security)
	}
}

func TestLoadToolsOperationSecurityOverridesGlobal(t *testing.T) {
	spec := `openapi: "3.0.3"
info:
  title: security override fixture
  version: "1.0.0"
components:
  securitySchemes:
    bearer:
      type: http
      scheme: bearer
    apiKey:
      type: apiKey
      in: header
      name: X-API-Key
security:
  - bearer: []
paths:
  /public:
    get:
      operationId: getPublic
      security: []
      responses:
        "200": { description: ok }
  /admin:
    get:
      operationId: getAdmin
      security:
        - apiKey: []
      responses:
        "200": { description: ok }
`
	path := writeTempSpec(t, spec)
	tools, err := openapi.LoadTools(path)
	if err != nil {
		t.Fatal(err)
	}
	pub := findRoute(t, tools, "getPublic")
	if len(pub.Security) != 0 {
		t.Fatalf("getPublic Security=%v, want empty (overrides global)", pub.Security)
	}
	admin := findRoute(t, tools, "getAdmin")
	if len(admin.Security) != 1 || admin.Security[0] != "apiKey" {
		t.Fatalf("getAdmin Security=%v, want [apiKey]", admin.Security)
	}
}

func TestInvokerInvokeWithHeadersOverlays(t *testing.T) {
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	inv := &openapi.Invoker{
		BaseURL: srv.URL,
		Tools: []openapi.ToolRoute{{
			Name:   "getMe",
			Method: http.MethodGet,
			Path:   "/me",
		}},
		Headers: map[string]string{"Authorization": "Bearer ENV"},
	}
	res, err := inv.InvokeWithHeaders(context.Background(), "getMe", nil, map[string]string{
		"Authorization": "Bearer CAPTURED",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
	if sawAuth != "Bearer CAPTURED" {
		t.Fatalf("Authorization=%q, want Bearer CAPTURED", sawAuth)
	}
	// Overlay must not mutate inv.Headers.
	if inv.Headers["Authorization"] != "Bearer ENV" {
		t.Fatalf("inv.Headers mutated: %v", inv.Headers)
	}
}

func writeTempSpec(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "openapi.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func findRoute(t *testing.T, tools []openapi.ToolRoute, name string) openapi.ToolRoute {
	t.Helper()
	for _, tl := range tools {
		if tl.Name == name {
			return tl
		}
	}
	t.Fatalf("tool %q not found", name)
	return openapi.ToolRoute{}
}

func TestLoadToolsFromSpec(t *testing.T) {
	tools, err := openapi.LoadTools("../../../examples/mock-ticket/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name] = true
	}
	if !names["create_ticket"] || !names["list_tickets"] {
		t.Fatalf("tools=%v", names)
	}
	var create openapi.ToolRoute
	for _, tl := range tools {
		if tl.Name == "create_ticket" {
			create = tl
		}
	}
	if create.Method != http.MethodPost || create.Path != "/tickets" {
		t.Fatalf("route=%+v", create)
	}
	if create.InputSchema == nil {
		t.Fatal("create_ticket InputSchema is nil")
	}
	props, _ := create.InputSchema["properties"].(map[string]any)
	if props == nil {
		t.Fatalf("properties missing: %+v", create.InputSchema)
	}
	title, _ := props["title"].(map[string]any)
	if title == nil || title["type"] != "string" {
		t.Fatalf("title schema=%v", title)
	}
	if _, ok := props["priority"]; !ok {
		t.Fatalf("priority missing: %+v", props)
	}
}

func TestLoadToolsOperationID(t *testing.T) {
	tools, err := openapi.LoadTools("../../../examples/mock-ticket/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var create openapi.ToolRoute
	found := false
	for _, tl := range tools {
		if tl.Name == "create_ticket" {
			create = tl
			found = true
			break
		}
	}
	if !found {
		t.Fatal("create_ticket not found")
	}
	if create.OperationID != "create_ticket" {
		t.Fatalf("OperationID=%q, want create_ticket", create.OperationID)
	}
	if create.Method != http.MethodPost {
		t.Fatalf("Method=%q, want POST", create.Method)
	}
}

func TestInvokerCreateTicket(t *testing.T) {
	srv := httptest.NewServer(mockticket.NewHandler())
	t.Cleanup(srv.Close)

	tools, err := openapi.LoadTools("../../../examples/mock-ticket/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	inv := &openapi.Invoker{BaseURL: srv.URL, Tools: tools}
	res, err := inv.Invoke(context.Background(), "create_ticket", map[string]any{
		"title":    "VPN 挂了",
		"priority": "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res)
	}
	if res.Content["id"] == nil || res.Content["title"] != "VPN 挂了" {
		t.Fatalf("content=%v", res.Content)
	}
}

func TestInvokerNon2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	inv := &openapi.Invoker{
		BaseURL: srv.URL,
		Tools: []openapi.ToolRoute{{
			Name:   "create_ticket",
			Method: http.MethodPost,
			Path:   "/tickets",
		}},
	}
	res, err := inv.Invoke(context.Background(), "create_ticket", map[string]any{"title": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError, got %+v", res)
	}
}

func TestLoadToolsPathParamOps(t *testing.T) {
	tools, err := openapi.LoadTools("../../../examples/mock-ticket/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]openapi.ToolRoute{}
	for _, tl := range tools {
		names[tl.Name] = tl
	}
	get, ok := names["get_ticket"]
	if !ok {
		t.Fatal("get_ticket missing")
	}
	if get.Method != http.MethodGet || get.Path != "/tickets/{id}" {
		t.Fatalf("get_ticket=%+v", get)
	}
	assertSchemaHasRequiredProp(t, get.InputSchema, "id", "string")
	if _, ok := get.InputSchema["properties"].(map[string]any)["status"]; ok {
		t.Fatalf("get_ticket should not have status: %+v", get.InputSchema)
	}

	patch, ok := names["update_ticket_status"]
	if !ok {
		t.Fatal("update_ticket_status missing")
	}
	if patch.Method != http.MethodPatch || patch.Path != "/tickets/{id}" {
		t.Fatalf("update_ticket_status=%+v", patch)
	}
	assertSchemaHasRequiredProp(t, patch.InputSchema, "id", "string")
	assertSchemaHasRequiredProp(t, patch.InputSchema, "status", "string")
}

func assertSchemaHasRequiredProp(t *testing.T, schema map[string]any, name, typ string) {
	t.Helper()
	if schema == nil {
		t.Fatalf("schema nil, want prop %q", name)
	}
	props, _ := schema["properties"].(map[string]any)
	prop, _ := props[name].(map[string]any)
	if prop == nil || prop["type"] != typ {
		t.Fatalf("prop %q=%v, want type %q in %+v", name, prop, typ, schema)
	}
	req, _ := schema["required"].([]any)
	found := false
	for _, r := range req {
		if r == name {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("required missing %q: %+v", name, schema)
	}
}

func TestInvokerGetTicketByID(t *testing.T) {
	srv := httptest.NewServer(mockticket.NewHandler())
	t.Cleanup(srv.Close)

	tools, err := openapi.LoadTools("../../../examples/mock-ticket/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	inv := &openapi.Invoker{BaseURL: srv.URL, Tools: tools}

	created, err := inv.Invoke(context.Background(), "create_ticket", map[string]any{
		"title": "path param get",
	})
	if err != nil || created.IsError {
		t.Fatalf("create: err=%v res=%+v", err, created)
	}
	id, _ := created.Content["id"].(string)
	if id == "" {
		t.Fatalf("missing id: %v", created.Content)
	}

	got, err := inv.Invoke(context.Background(), "get_ticket", map[string]any{"id": id})
	if err != nil {
		t.Fatal(err)
	}
	if got.IsError {
		t.Fatalf("get IsError: %+v", got)
	}
	if got.Content["id"] != id {
		t.Fatalf("content=%v", got.Content)
	}
}

func TestInvokerPatchTicketStatus(t *testing.T) {
	var sawPath string
	var sawBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		if r.Method == http.MethodPatch {
			_ = json.NewDecoder(r.Body).Decode(&sawBody)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"ticket_1","status":"closed"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	inv := &openapi.Invoker{
		BaseURL: srv.URL,
		Tools: []openapi.ToolRoute{{
			Name:   "update_ticket_status",
			Method: http.MethodPatch,
			Path:   "/tickets/{id}",
		}},
	}
	res, err := inv.Invoke(context.Background(), "update_ticket_status", map[string]any{
		"id":     "ticket_1",
		"status": "closed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %+v", res)
	}
	if sawPath != "/tickets/ticket_1" {
		t.Fatalf("path=%q, want /tickets/ticket_1", sawPath)
	}
	if _, hasID := sawBody["id"]; hasID {
		t.Fatalf("path param id must not be in body: %v", sawBody)
	}
	if sawBody["status"] != "closed" {
		t.Fatalf("body=%v", sawBody)
	}
}
