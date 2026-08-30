package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func mcpExportServer(t *testing.T) *Server {
	t.Helper()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	st.UpsertTool(store.Tool{
		ConnectorID: "c1",
		Name:        "get_status",
		Source:      store.ToolSourceSpec,
		Enabled:     true,
		Method:      "GET",
		Description: "status",
		Export:      "default",
	})
	reg.Register("get_status", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	srv := NewServer(st, reg, &gateFakeRunner{store: st})
	srv.OperatorToken = "op-token"
	srv.AdminToken = "adm-token"
	srv.MCPExportEnabled = true
	srv.Identities = identity.NewMemoryStore()
	return srv
}

func withAdmin(req *http.Request) *http.Request {
	req.Header.Set("Authorization", "Bearer adm-token")
	return req
}

func withOperator(req *http.Request) *http.Request {
	req.Header.Set("Authorization", "Bearer op-token")
	return req
}

func TestMCPExportAdminCRUDAndACL(t *testing.T) {
	srv := mcpExportServer(t)
	h := srv.Handler()

	// operator cannot manage
	req := withOperator(httptest.NewRequest(http.MethodGet, "/v0/settings/mcp-export", nil))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operator GET settings: code=%d body=%s", rr.Code, rr.Body.String())
	}

	// admin settings
	req = withAdmin(httptest.NewRequest(http.MethodGet, "/v0/settings/mcp-export", nil))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin GET settings: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var settings struct {
		Enabled      bool   `json:"enabled"`
		EndpointPath string `json:"endpoint_path"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	if !settings.Enabled || settings.EndpointPath != "/v0/mcp/export" {
		t.Fatalf("settings=%+v", settings)
	}

	// create identity
	req = withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/mcp-export/identities",
		strings.NewReader(`{"name":"Ops","scheme":"Bearer","headers":{"X-Team":"ops"}}`)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create identity: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var identity store.MCPExportIdentity
	if err := json.Unmarshal(rr.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if identity.ID == "" || identity.Name != "Ops" {
		t.Fatalf("identity=%+v", identity)
	}

	// POST key without identity_id → 400
	req = withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/mcp-export/keys",
		strings.NewReader(`{"name":"k1"}`)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("key without identity_id: code=%d", rr.Code)
	}

	// create key
	req = withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/mcp-export/keys",
		strings.NewReader(`{"name":"k1","identity_id":"`+identity.ID+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create key: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		IdentityID string `json:"identity_id"`
		Token      string `json:"token"`
		Prefix     string `json:"prefix"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.ID == "" || created.IdentityID != identity.ID {
		t.Fatalf("created=%+v", created)
	}
	plaintext := created.Token

	// list keys has no token
	req = withAdmin(httptest.NewRequest(http.MethodGet, "/v0/settings/mcp-export/keys", nil))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list keys: code=%d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), plaintext) || strings.Contains(rr.Body.String(), `"token"`) {
		t.Fatalf("list must not include plaintext token: %s", rr.Body.String())
	}

	// export key can call /v0/mcp/export
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	ctx := context.Background()
	session, err := mcpbridge.ConnectHTTP(ctx, ts.URL+"/v0/mcp/export", map[string]string{
		"Authorization": "Bearer " + plaintext,
	})
	if err != nil {
		t.Fatalf("ConnectHTTP with export key: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	listed, err := mcpbridge.ListTools(ctx, session)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := false
	for _, tl := range listed.Tools {
		if tl.Name == "get_status" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected get_status, got %#v", listed.Tools)
	}

	// Gate admin token cannot authenticate as MCP key
	req = httptest.NewRequest(http.MethodPost, "/v0/mcp/export", nil)
	req.Header.Set("Authorization", "Bearer adm-token")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("gate token on export: code=%d body=%s", rr.Code, rr.Body.String())
	}

	// export key cannot call admin API
	req = httptest.NewRequest(http.MethodGet, "/v0/settings/mcp-export", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("export key on admin API: code=%d body=%s", rr.Code, rr.Body.String())
	}

	// revoke key
	req = withAdmin(httptest.NewRequest(http.MethodDelete, "/v0/settings/mcp-export/keys/"+created.ID, nil))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke key: code=%d body=%s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v0/mcp/export", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key: code=%d", rr.Code)
	}

	// patch + delete identity
	req = withAdmin(httptest.NewRequest(http.MethodPatch, "/v0/settings/mcp-export/identities/"+identity.ID,
		strings.NewReader(`{"name":"Ops2"}`)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"Ops2"`) {
		t.Fatalf("patch identity: code=%d body=%s", rr.Code, rr.Body.String())
	}
	req = withAdmin(httptest.NewRequest(http.MethodDelete, "/v0/settings/mcp-export/identities/"+identity.ID, nil))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete identity: code=%d body=%s", rr.Code, rr.Body.String())
	}

	_ = io.Discard
}

func TestMCPExportRoleNoneKeepsAuthorization(t *testing.T) {
	srv := mcpExportServer(t)
	// Seed a key directly so we can prove Authorization survived authorize().
	id := store.MCPExportIdentity{ID: "idt_auth", Name: "A"}
	if err := srv.Store.UpsertMCPExportIdentity(id); err != nil {
		t.Fatal(err)
	}
	// Create via admin API to get plaintext.
	req := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/mcp-export/keys",
		strings.NewReader(`{"name":"k","identity_id":"idt_auth"}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create key: %d %s", rr.Code, rr.Body.String())
	}
	var created struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &created)
	if created.Token == "" {
		t.Fatal("missing token")
	}

	req = httptest.NewRequest(http.MethodPost, "/v0/mcp/export", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+created.Token)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	// Auth succeeded enough to leave 401; any non-401 proves header was kept
	// (invalid JSON-RPC may still be 4xx from MCP layer, but not 401).
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("Authorization stripped or key rejected: code=%d body=%s", rr.Code, rr.Body.String())
	}
}