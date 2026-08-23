package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func readOpenAPIFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "examples", "spec-import", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func openAPIServer(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := NewServer(st, reg, &gateFakeRunner{store: st})
	srv.Identities = identity.NewMemoryStore()
	srv.DataDir = t.TempDir()
	return srv, srv.Handler(), srv.DataDir
}

func TestOpenAPIPutSpecContentDiscoversTools(t *testing.T) {
	_, h, _ := openAPIServer(t)
	specContent := readOpenAPIFixture(t, "openapi3-min.json")

	putBody := map[string]any{
		"type":          "openapi",
		"base_url":      "https://api.example.com",
		"spec_content":  specContent,
		"import_format": "openapi3",
	}
	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/demo", jsonBodyAPI(t, putBody))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", rr.Code, rr.Body.String())
	}

	var putResp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&putResp); err != nil {
		t.Fatal(err)
	}
	if putResp["import_format_detected"] != "openapi3" {
		t.Fatalf("import_format_detected=%v want openapi3", putResp["import_format_detected"])
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v0/connectors/demo", nil)
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var getResp map[string]any
	if err := json.NewDecoder(getRR.Body).Decode(&getResp); err != nil {
		t.Fatal(err)
	}
	if getResp["import_format_detected"] != "openapi3" {
		t.Fatalf("GET import_format_detected=%v want openapi3", getResp["import_format_detected"])
	}

	toolsReq := httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	toolsRR := httptest.NewRecorder()
	h.ServeHTTP(toolsRR, toolsReq)
	if toolsRR.Code != http.StatusOK {
		t.Fatalf("GET /v0/tools status=%d body=%s", toolsRR.Code, toolsRR.Body.String())
	}
	var toolsBody struct {
		Tools []store.Tool `json:"tools"`
	}
	if err := json.NewDecoder(toolsRR.Body).Decode(&toolsBody); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range toolsBody.Tools {
		if tool.Name == "list_items" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected list_items in tools: %+v", toolsBody.Tools)
	}
}

func TestOpenAPIPutEditBaseURLWithoutSpecContent(t *testing.T) {
	_, h, _ := openAPIServer(t)
	specContent := readOpenAPIFixture(t, "openapi3-min.json")

	putBody := map[string]any{
		"type":          "openapi",
		"base_url":      "https://api.example.com",
		"spec_content":  specContent,
		"import_format": "openapi3",
	}
	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/demo", jsonBodyAPI(t, putBody))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("initial PUT status=%d body=%s", rr.Code, rr.Body.String())
	}

	editBody := map[string]any{
		"type":     "openapi",
		"base_url": "https://api.example.org",
	}
	editReq := httptest.NewRequest(http.MethodPut, "/v0/connectors/demo", jsonBodyAPI(t, editBody))
	editRR := httptest.NewRecorder()
	h.ServeHTTP(editRR, editReq)
	if editRR.Code != http.StatusOK {
		t.Fatalf("edit PUT status=%d body=%s", editRR.Code, editRR.Body.String())
	}

	var editResp map[string]any
	if err := json.NewDecoder(editRR.Body).Decode(&editResp); err != nil {
		t.Fatal(err)
	}
	if editResp["base_url"] != "https://api.example.org" {
		t.Fatalf("base_url=%v want https://api.example.org", editResp["base_url"])
	}
	if editResp["import_format_detected"] != "openapi3" {
		t.Fatalf("import_format_detected=%v want openapi3", editResp["import_format_detected"])
	}

	toolsReq := httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	toolsRR := httptest.NewRecorder()
	h.ServeHTTP(toolsRR, toolsReq)
	if toolsRR.Code != http.StatusOK {
		t.Fatalf("GET /v0/tools status=%d body=%s", toolsRR.Code, toolsRR.Body.String())
	}
	var toolsBody struct {
		Tools []store.Tool `json:"tools"`
	}
	if err := json.NewDecoder(toolsRR.Body).Decode(&toolsBody); err != nil {
		t.Fatal(err)
	}
	if len(toolsBody.Tools) == 0 {
		t.Fatal("expected tools after base_url-only edit")
	}
}

func TestOpenAPIPutUnsupportedImportFormat(t *testing.T) {
	_, h, _ := openAPIServer(t)
	putBody := map[string]any{
		"type":          "openapi",
		"base_url":      "https://api.example.com",
		"spec_content":  readOpenAPIFixture(t, "openapi3-min.json"),
		"import_format": "raml",
	}
	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/demo", jsonBodyAPI(t, putBody))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
	var errBody errorBody
	if err := json.NewDecoder(rr.Body).Decode(&errBody); err != nil {
		t.Fatal(err)
	}
	if errBody.Error.Code != "unsupported_import_format" {
		t.Fatalf("code=%q want unsupported_import_format", errBody.Error.Code)
	}
}
