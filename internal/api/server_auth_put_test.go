package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func writeTicketSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	specPath := filepath.Join(dir, "openapi.yaml")
	if err := os.WriteFile(specPath, []byte(`openapi: 3.0.3
info: { title: t, version: 0.1.0 }
paths:
  /tickets:
    post:
      operationId: create_ticket
      responses: { "201": { description: created } }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	return specPath
}

func TestPutConnectorVaultRefPersistsAuthShape(t *testing.T) {
	t.Setenv("PUT_VAULT_TOK", "secret-xyz")
	spec := writeTicketSpec(t)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":     "openapi",
			"spec":     spec,
			"base_url": "http://x",
			"auth": map[string]any{
				"mode": "vault_ref",
				"vault_ref": map[string]any{
					"headers": map[string]string{"Authorization": "env:PUT_VAULT_TOK"},
				},
			},
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "secret-xyz") {
		t.Fatalf("leaked secret in PUT response: %s", body)
	}
	if !strings.Contains(body, "env:PUT_VAULT_TOK") {
		t.Fatalf("want ref in body=%s", body)
	}

	get := httptest.NewRequest(http.MethodGet, "/v0/connectors/ticket", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", rr.Code, rr.Body.String())
	}
	getBody := rr.Body.String()
	if !strings.Contains(getBody, "vault_ref") || !strings.Contains(getBody, "env:PUT_VAULT_TOK") {
		t.Fatalf("get missing auth shape: %s", getBody)
	}
	if strings.Contains(getBody, "secret-xyz") {
		t.Fatalf("get leaked secret: %s", getBody)
	}

	c, err := st.GetConnector("ticket")
	if err != nil {
		t.Fatal(err)
	}
	if c.Auth.Mode != "vault_ref" {
		t.Fatalf("c.Auth.Mode=%q", c.Auth.Mode)
	}
	if c.Auth.VaultRef.Headers["Authorization"] != "env:PUT_VAULT_TOK" {
		t.Fatalf("c.Auth.VaultRef=%+v", c.Auth.VaultRef)
	}
	if srv.AuthMode != "vault_ref" {
		t.Fatalf("srv.AuthMode=%q want vault_ref", srv.AuthMode)
	}
}

func TestPutConnectorStaticAuthPersistsShape(t *testing.T) {
	t.Setenv("PUT_STATIC_TOK", "static-tok-val")
	spec := writeTicketSpec(t)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":     "openapi",
			"spec":     spec,
			"base_url": "http://x",
			"auth": map[string]any{
				"mode": "static",
				"static": map[string]any{
					"headers": map[string]string{"Authorization": "Bearer ${PUT_STATIC_TOK}"},
				},
			},
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "static-tok-val") {
		t.Fatalf("leaked resolved secret: %s", body)
	}
	if !strings.Contains(body, "${PUT_STATIC_TOK}") {
		t.Fatalf("want static ref in body=%s", body)
	}
	if srv.AuthMode != "static" {
		t.Fatalf("srv.AuthMode=%q want static", srv.AuthMode)
	}
}

func TestPutConnectorInvalidAuthDoesNotRegister(t *testing.T) {
	spec := writeTicketSpec(t)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":     "openapi",
			"spec":     spec,
			"base_url": "http://x",
			"auth": map[string]any{
				"mode": "vault_ref",
				"vault_ref": map[string]any{
					"headers": map[string]string{"Authorization": "env:PUT_MISSING_VAULT_TOK_XYZ"},
				},
			},
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "invalid_auth" {
		t.Fatalf("code=%q want invalid_auth", wrap.Error.Code)
	}
	if len(reg.List()) != 0 {
		t.Fatalf("registry not empty after invalid_auth: %+v", reg.List())
	}
	if _, err := st.GetConnector("ticket"); err == nil {
		t.Fatal("store should not contain connector after invalid_auth")
	}
}

func TestPutConnectorUnknownAuthModeDoesNotRegister(t *testing.T) {
	spec := writeTicketSpec(t)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":     "openapi",
			"spec":     spec,
			"base_url": "http://x",
			"auth":     map[string]any{"mode": "ldap"},
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "invalid_auth" {
		t.Fatalf("code=%q want invalid_auth", wrap.Error.Code)
	}
	if len(reg.List()) != 0 {
		t.Fatalf("registry not empty after unknown mode: %+v", reg.List())
	}
}

func TestPutConnectorPassthroughConfiguresServerWhitelist(t *testing.T) {
	spec := writeTicketSpec(t)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket",
		jsonBody(t, map[string]any{
			"type":     "openapi",
			"spec":     spec,
			"base_url": "http://x",
			"auth": map[string]any{
				"mode": "passthrough",
				"passthrough": map[string]any{
					"headers": []string{"X-Custom-Auth", "X-Trace-Id"},
				},
			},
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if srv.AuthMode != "passthrough" {
		t.Fatalf("srv.AuthMode=%q want passthrough", srv.AuthMode)
	}
	if len(srv.AuthWhitelist) != 2 ||
		srv.AuthWhitelist[0] != "X-Custom-Auth" ||
		srv.AuthWhitelist[1] != "X-Trace-Id" {
		t.Fatalf("srv.AuthWhitelist=%+v", srv.AuthWhitelist)
	}
}
