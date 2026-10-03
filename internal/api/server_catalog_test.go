package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func writeCatalogSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "openapi.yaml")
	content := `openapi: 3.0.3
info:
  title: catalog
  version: 0.1.0
paths:
  /tickets:
    post:
      operationId: create_ticket
      responses:
        "201":
          description: created
  /tickets/{id}:
    get:
      operationId: get_ticket
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

func catalogServer(t *testing.T) (*Server, *tool.Registry, store.Store, http.Handler) {
	t.Helper()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := NewServer(st, reg, &gateFakeRunner{store: st})
	srv.Identities = identity.NewMemoryStore()
	return srv, reg, st, srv.Handler()
}

func jsonBodyAPI(t *testing.T, v any) *strings.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReader(string(b))
}

func putCatalogConnector(t *testing.T, h http.Handler, id, spec, baseURL string, extra ...map[string]any) {
	t.Helper()
	body := map[string]any{
		"type":     "openapi",
		"spec":     spec,
		"base_url": baseURL,
	}
	for _, e := range extra {
		for k, v := range e {
			body[k] = v
		}
	}
	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/"+id, jsonBodyAPI(t, body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT connector %s status=%d body=%s", id, rr.Code, rr.Body.String())
	}
}

func getToolsList(t *testing.T, h http.Handler) []store.Tool {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /v0/tools status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Tools []store.Tool `json:"tools"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Tools
}

func findTool(tools []store.Tool, name string) (store.Tool, bool) {
	for _, t := range tools {
		if t.Name == name {
			return t, true
		}
	}
	return store.Tool{}, false
}

func TestCatalogGetIncludesEnabledAndSource(t *testing.T) {
	spec := writeCatalogSpec(t)
	_, _, _, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec, "http://127.0.0.1:9")

	tools := getToolsList(t, h)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d: %+v", len(tools), tools)
	}
	for _, tt := range tools {
		if !tt.Enabled {
			t.Fatalf("spec tool %q must default to enabled=true: %+v", tt.Name, tt)
		}
		if tt.Source != store.ToolSourceSpec {
			t.Fatalf("spec tool %q source=%q want spec", tt.Name, tt.Source)
		}
		if tt.Method == "" || tt.Path == "" {
			t.Fatalf("spec tool %q missing method/path: %+v", tt.Name, tt)
		}
	}
}

func TestCatalogPatchEnabledDisablesAndKeepsRow(t *testing.T) {
	spec := writeCatalogSpec(t)
	_, reg, _, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec, "http://127.0.0.1:9")

	patch := httptest.NewRequest(http.MethodPatch, "/v0/tools/get_ticket",
		jsonBodyAPI(t, map[string]any{"enabled": false}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, patch)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH enabled=false status=%d body=%s", rr.Code, rr.Body.String())
	}
	var row store.Tool
	if err := json.NewDecoder(rr.Body).Decode(&row); err != nil {
		t.Fatal(err)
	}
	if row.Enabled {
		t.Fatalf("PATCH response must have enabled=false: %+v", row)
	}
	if row.Source != store.ToolSourceSpec {
		t.Fatalf("PATCH response source=%q want spec", row.Source)
	}
	if _, ok := reg.Get("get_ticket"); ok {
		t.Fatal("disabled tool must not be in Registry")
	}

	tools := getToolsList(t, h)
	gt, ok := findTool(tools, "get_ticket")
	if !ok {
		t.Fatal("disabled tool must still be listed in GET /v0/tools")
	}
	if gt.Enabled {
		t.Fatalf("GET /v0/tools must show enabled=false: %+v", gt)
	}

	patchLogin := httptest.NewRequest(http.MethodPatch, "/v0/tools/get_ticket",
		jsonBodyAPI(t, map[string]any{"require_login": true}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, patchLogin)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH require_login on disabled status=%d body=%s", rr.Code, rr.Body.String())
	}
	tools = getToolsList(t, h)
	gt, _ = findTool(tools, "get_ticket")
	if !gt.RequireLogin {
		t.Fatalf("require_login must be persisted on disabled row: %+v", gt)
	}
	if gt.Enabled {
		t.Fatalf("row must still be disabled: %+v", gt)
	}
}

func TestCatalogPatchEnabledReRegisters(t *testing.T) {
	spec := writeCatalogSpec(t)
	_, reg, _, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec, "http://127.0.0.1:9")

	patch := httptest.NewRequest(http.MethodPatch, "/v0/tools/get_ticket",
		jsonBodyAPI(t, map[string]any{"enabled": false}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, patch)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH off status=%d body=%s", rr.Code, rr.Body.String())
	}

	patchOn := httptest.NewRequest(http.MethodPatch, "/v0/tools/get_ticket",
		jsonBodyAPI(t, map[string]any{"enabled": true}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, patchOn)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH on status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, ok := reg.Get("get_ticket"); !ok {
		t.Fatal("re-enabled tool must be back in Registry")
	}
}

func TestCatalogPostExtraRegistersAndDispatches(t *testing.T) {
	spec := writeCatalogSpec(t)
	var gotMethod, gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	_, reg, _, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec, upstream.URL)

	post := httptest.NewRequest(http.MethodPost, "/v0/connectors/c1/tools",
		jsonBodyAPI(t, map[string]any{
			"name":         "get_device",
			"description":  "按 id 查设备",
			"method":       "GET",
			"path":         "/devices/{id}",
			"input_schema": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}},
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST extra status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created store.Tool
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Source != store.ToolSourceExtra || !created.Enabled {
		t.Fatalf("POST response must be extra+enabled: %+v", created)
	}

	tools := getToolsList(t, h)
	gd, ok := findTool(tools, "get_device")
	if !ok {
		t.Fatal("GET /v0/tools must contain the extra")
	}
	if gd.Source != store.ToolSourceExtra {
		t.Fatalf("extra source=%q want extra", gd.Source)
	}
	if _, ok := reg.Get("get_device"); !ok {
		t.Fatal("extra must be registered in Registry")
	}

	gotMethod, gotPath = "", ""
	_, isErr, err := reg.Invoke(context.Background(), "get_device", map[string]any{"id": "x"})
	if err != nil || isErr {
		t.Fatalf("invoke get_device: isErr=%v err=%v", isErr, err)
	}
	if gotMethod != http.MethodGet || gotPath != "/devices/x" {
		t.Fatalf("downstream got %s %q, want GET /devices/x", gotMethod, gotPath)
	}
}

func TestCatalogDeleteSpecReturns400AndDeleteExtraWorks(t *testing.T) {
	spec := writeCatalogSpec(t)
	_, reg, _, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec, "http://127.0.0.1:9")

	post := httptest.NewRequest(http.MethodPost, "/v0/connectors/c1/tools",
		jsonBodyAPI(t, map[string]any{"name": "extra1", "method": "GET", "path": "/extra1"}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST extra status=%d body=%s", rr.Code, rr.Body.String())
	}

	delSpec := httptest.NewRequest(http.MethodDelete, "/v0/connectors/c1/tools/create_ticket", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, delSpec)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("DELETE spec status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "invalid_request" {
		t.Fatalf("DELETE spec code=%q want invalid_request", wrap.Error.Code)
	}

	delExtra := httptest.NewRequest(http.MethodDelete, "/v0/connectors/c1/tools/extra1", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, delExtra)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("DELETE extra status=%d want 204 body=%s", rr.Code, rr.Body.String())
	}
	tools := getToolsList(t, h)
	if _, ok := findTool(tools, "extra1"); ok {
		t.Fatal("extra must be gone from GET /v0/tools after DELETE")
	}
	if _, ok := reg.Get("extra1"); ok {
		t.Fatal("extra must be unregistered from Registry after DELETE")
	}
}

func TestCatalogSecondPutPreservesDisabledAndExtra(t *testing.T) {
	spec := writeCatalogSpec(t)
	_, reg, _, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec, "http://127.0.0.1:9")

	patch := httptest.NewRequest(http.MethodPatch, "/v0/tools/get_ticket",
		jsonBodyAPI(t, map[string]any{"enabled": false}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, patch)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH off status=%d body=%s", rr.Code, rr.Body.String())
	}

	post := httptest.NewRequest(http.MethodPost, "/v0/connectors/c1/tools",
		jsonBodyAPI(t, map[string]any{"name": "extra1", "method": "GET", "path": "/extra1"}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST extra status=%d body=%s", rr.Code, rr.Body.String())
	}

	putCatalogConnector(t, h, "c1", spec, "http://127.0.0.1:9")

	tools := getToolsList(t, h)
	gt, ok := findTool(tools, "get_ticket")
	if !ok {
		t.Fatal("disabled spec row must survive second PUT")
	}
	if gt.Enabled {
		t.Fatalf("disabled row must stay disabled after second PUT: %+v", gt)
	}
	if _, ok := findTool(tools, "extra1"); !ok {
		t.Fatal("extra must survive second PUT")
	}
	ct, ok := findTool(tools, "create_ticket")
	if !ok || !ct.Enabled {
		t.Fatalf("create_ticket must be enabled after second PUT: %+v", ct)
	}
	if _, ok := reg.Get("create_ticket"); !ok {
		t.Fatal("create_ticket must be registered after second PUT")
	}
	if _, ok := reg.Get("get_ticket"); ok {
		t.Fatal("disabled get_ticket must not be registered after second PUT")
	}
	if _, ok := reg.Get("extra1"); !ok {
		t.Fatal("extra must be registered after second PUT")
	}
}

func TestCatalogPluginPostExtraReturns400(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"echo"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer sidecar.Close()

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := NewServer(st, reg, &gateFakeRunner{store: st})
	srv.Identities = identity.NewMemoryStore()
	h := srv.Handler()

	put := httptest.NewRequest(http.MethodPut, "/v0/connectors/side",
		jsonBodyAPI(t, map[string]any{"type": "http", "base_url": sidecar.URL}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT plugin status=%d body=%s", rr.Code, rr.Body.String())
	}

	post := httptest.NewRequest(http.MethodPost, "/v0/connectors/side/tools",
		jsonBodyAPI(t, map[string]any{"name": "phantom", "method": "GET", "path": "/phantom"}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("POST extra on plugin status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "invalid_request" {
		t.Fatalf("code=%q want invalid_request", wrap.Error.Code)
	}
	if _, ok := reg.Get("phantom"); ok {
		t.Fatal("phantom must not be registered on a plugin connector")
	}
}

func TestCatalogDuplicatePostReturns409(t *testing.T) {
	spec := writeCatalogSpec(t)
	_, _, _, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec, "http://127.0.0.1:9")

	post := httptest.NewRequest(http.MethodPost, "/v0/connectors/c1/tools",
		jsonBodyAPI(t, map[string]any{"name": "create_ticket", "method": "GET", "path": "/x"}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != http.StatusConflict {
		t.Fatalf("duplicate POST status=%d want 409 body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "conflict" {
		t.Fatalf("code=%q want conflict", wrap.Error.Code)
	}
}

func TestCatalogPatchBothMissingReturns400(t *testing.T) {
	spec := writeCatalogSpec(t)
	_, _, _, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec, "http://127.0.0.1:9")

	patch := httptest.NewRequest(http.MethodPatch, "/v0/tools/create_ticket",
		jsonBodyAPI(t, map[string]any{}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, patch)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("{} PATCH status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "invalid_request" {
		t.Fatalf("code=%q want invalid_request", wrap.Error.Code)
	}
}

func TestCatalogGetConnectorIncludesDisabled(t *testing.T) {
	spec := writeCatalogSpec(t)
	_, _, _, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec, "http://127.0.0.1:9")

	patch := httptest.NewRequest(http.MethodPatch, "/v0/tools/get_ticket",
		jsonBodyAPI(t, map[string]any{"enabled": false}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, patch)
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH off status=%d body=%s", rr.Code, rr.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/v0/connectors/c1", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET connector status=%d body=%s", rr.Code, rr.Body.String())
	}
	var connBody struct {
		Tools []store.Tool `json:"tools"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&connBody); err != nil {
		t.Fatal(err)
	}
	gt, ok := findTool(connBody.Tools, "get_ticket")
	if !ok {
		t.Fatal("GET /v0/connectors/{id} tools must include the disabled row")
	}
	if gt.Enabled {
		t.Fatalf("disabled row must show enabled=false in connector tools: %+v", gt)
	}
}

func TestCatalogPatchTitleAndDescription(t *testing.T) {
	dir := t.TempDir()
	spec1 := filepath.Join(dir, "a.yaml")
	spec2 := filepath.Join(dir, "b.yaml")
	body1 := `openapi: 3.0.3
info: {title: c, version: "0.1.0"}
paths:
  /tickets:
    post:
      operationId: create_ticket
      responses: {"201": {description: created}}
  /tickets/{id}:
    get:
      operationId: get_ticket
      description: spec-one
      parameters: [{name: id, in: path, required: true, schema: {type: string}}]
      responses: {"200": {description: ok}}
`
	body2 := strings.Replace(body1, "spec-one", "spec-two", 1)
	if err := os.WriteFile(spec1, []byte(body1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec2, []byte(body2), 0o644); err != nil {
		t.Fatal(err)
	}
	_, reg, _, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec1, "http://127.0.0.1:9")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/v0/tools/get_ticket",
		jsonBodyAPI(t, map[string]any{"title": "查工单", "description": "人改说明"})))
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH title+desc status=%d body=%s", rr.Code, rr.Body.String())
	}
	tools := getToolsList(t, h)
	gt, _ := findTool(tools, "get_ticket")
	if gt.Title != "查工单" || gt.Description != "人改说明" || !gt.DescriptionCustom {
		t.Fatalf("after patch=%+v", gt)
	}
	info, ok := reg.Get("get_ticket")
	if !ok || info.Description != "人改说明" {
		t.Fatalf("registry desc=%+v ok=%v", info, ok)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/v0/tools/get_ticket",
		jsonBodyAPI(t, map[string]any{"title": "只改标题"})))
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH title-only status=%d", rr.Code)
	}
	info, _ = reg.Get("get_ticket")
	if info.Description != "人改说明" {
		t.Fatalf("title-only must not change registry desc: %+v", info)
	}

	putCatalogConnector(t, h, "c1", spec2, "http://127.0.0.1:9")
	tools = getToolsList(t, h)
	gt, _ = findTool(tools, "get_ticket")
	if gt.Title != "只改标题" || gt.Description != "人改说明" || !gt.DescriptionCustom {
		t.Fatalf("after re-PUT=%+v", gt)
	}
	info, _ = reg.Get("get_ticket")
	if info.Description != "人改说明" {
		t.Fatalf("registry after re-PUT=%+v", info)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/v0/tools/get_ticket", jsonBodyAPI(t, map[string]any{})))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty PATCH want 400 got %d", rr.Code)
	}
}

func TestCatalogPostExtraSetsTitleAndCustom(t *testing.T) {
	spec := writeCatalogSpec(t)
	_, _, _, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec, "http://127.0.0.1:9")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/connectors/c1/tools",
		jsonBodyAPI(t, map[string]any{
			"name": "echo_extra", "method": "GET", "path": "/echo",
			"title": "回声", "description": "手加",
		})))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST extra status=%d body=%s", rr.Code, rr.Body.String())
	}
	tools := getToolsList(t, h)
	ex, ok := findTool(tools, "echo_extra")
	if !ok || ex.Title != "回声" || !ex.DescriptionCustom || ex.Source != store.ToolSourceExtra {
		t.Fatalf("extra=%+v ok=%v", ex, ok)
	}
}
