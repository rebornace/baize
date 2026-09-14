package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/connector/mcpoauth"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/settingscrypto"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestMCPOAuthStartOperatorForbidden(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID:   "mcp1",
		Type: "mcp",
		MCP:  store.MCPConfig{Transport: "http", URL: "http://127.0.0.1:9/mcp"},
	})
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.OperatorToken = "op"
	srv.AdminToken = "adm"
	srv.CallbackPublicBase = "http://localhost:8080"

	req := httptest.NewRequest(http.MethodPost, "/v0/connectors/mcp1/mcp/oauth/start", nil)
	req.Header.Set("Authorization", "Bearer op")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operator start: status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestMCPOAuthStartRequiresPublicBase(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID:   "mcp1",
		Type: "mcp",
		MCP:  store.MCPConfig{Transport: "http", URL: "http://127.0.0.1:9/mcp"},
	})
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.CallbackPublicBase = ""

	req := httptest.NewRequest(http.MethodPost, "/v0/connectors/mcp1/mcp/oauth/start", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "public_base_required") {
		t.Fatalf("want public_base_required: %s", rr.Body.String())
	}
}

func TestMCPOAuthStartAfterPublicBaseHotPatch(t *testing.T) {
	setTestSettingsKey(t)
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID:   "mcp1",
		Type: "mcp",
		MCP:  store.MCPConfig{Transport: "http", URL: "http://127.0.0.1:9/mcp"},
	})
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.AdminToken = "adm"
	h := runtimecfg.New(runtimecfg.Snapshot{})
	srv.Settings = h
	srv.CallbackPublicBase = ""

	req := httptest.NewRequest(http.MethodPost, "/v0/connectors/mcp1/mcp/oauth/start", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "public_base_required") {
		t.Fatalf("before patch: status=%d body=%s", rr.Code, rr.Body.String())
	}

	patch := httptest.NewRequest(http.MethodPatch, "/v0/settings/runtime",
		strings.NewReader(`{"public_base_url":"http://127.0.0.1:8080"}`))
	patch.Header.Set("Authorization", "Bearer adm")
	patch.Header.Set("Content-Type", "application/json")
	pr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(pr, patch)
	if pr.Code != http.StatusOK {
		t.Fatalf("patch: status=%d body=%s", pr.Code, pr.Body.String())
	}
	if srv.CallbackPublicBase != "http://127.0.0.1:8080" {
		t.Fatalf("CallbackPublicBase=%q", srv.CallbackPublicBase)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/v0/connectors/mcp1/mcp/oauth/start", nil)
	req2.Header.Set("Authorization", "Bearer adm")
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	if strings.Contains(rr2.Body.String(), "public_base_required") {
		t.Fatalf("after patch still public_base_required: %s", rr2.Body.String())
	}
	// Discover will fail (no real MCP); any non-public_base error is success for this test.
	if rr2.Code == http.StatusOK {
		t.Fatal("unexpected OK without MCP AS")
	}
}

func TestMCPOAuthStartCallbackDisconnect(t *testing.T) {
	setTestSettingsKey(t)

	var asBase string
	mcpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/mcp":
			prm := asBase + "/.well-known/oauth-protected-resource"
			w.Header().Set("WWW-Authenticate", `Bearer realm="mcp", resource_metadata="`+prm+`"`)
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              asBase + "/mcp",
				"authorization_servers": []string{asBase},
			})
		case r.URL.Path == "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":                            asBase,
				"authorization_endpoint":            asBase + "/authorize",
				"token_endpoint":                    asBase + "/token",
				"registration_endpoint":             asBase + "/register",
				"code_challenge_methods_supported":  "S256",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/register":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"client_id": "dcr-cid",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/token":
			body, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(body))
			if form.Get("grant_type") != "authorization_code" || form.Get("code") == "" {
				http.Error(w, "bad token request", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-from-callback",
				"refresh_token": "refresh-from-callback",
				"token_type":    "Bearer",
				"expires_in":    3600,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mcpSrv.Close()
	asBase = mcpSrv.URL

	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID:   "oauth-http",
		Type: "mcp",
		MCP: store.MCPConfig{
			Transport: "http",
			URL:       mcpSrv.URL + "/mcp",
		},
	})
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.CallbackPublicBase = "http://localhost:18080"

	// Start → authorization_url with code_challenge.
	startReq := httptest.NewRequest(http.MethodPost, "/v0/connectors/oauth-http/mcp/oauth/start", nil)
	startRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(startRR, startReq)
	if startRR.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", startRR.Code, startRR.Body.String())
	}
	var startBody struct {
		AuthorizationURL string `json:"authorization_url"`
	}
	if err := json.NewDecoder(startRR.Body).Decode(&startBody); err != nil {
		t.Fatal(err)
	}
	authURL, err := url.Parse(startBody.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	q := authURL.Query()
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorization_url missing PKCE: %s", startBody.AuthorizationURL)
	}
	state := q.Get("state")
	if state == "" {
		t.Fatal("missing state")
	}
	if q.Get("client_id") != "dcr-cid" {
		t.Fatalf("client_id=%q", q.Get("client_id"))
	}

	// Wrong state → 400.
	badCB := httptest.NewRequest(http.MethodGet, "/v0/connectors/oauth-http/mcp/oauth/callback?code=x&state=wrong", nil)
	badRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(badRR, badCB)
	if badRR.Code != http.StatusBadRequest {
		t.Fatalf("bad state status=%d body=%s", badRR.Code, badRR.Body.String())
	}

	// Correct callback → authorized; GET redacts token_bundle.
	cb := httptest.NewRequest(http.MethodGet, "/v0/connectors/oauth-http/mcp/oauth/callback?code=auth-code&state="+url.QueryEscape(state), nil)
	cbRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(cbRR, cb)
	if cbRR.Code != http.StatusOK {
		t.Fatalf("callback status=%d body=%s", cbRR.Code, cbRR.Body.String())
	}
	if !strings.Contains(cbRR.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("callback content-type=%q", cbRR.Header().Get("Content-Type"))
	}

	c, err := st.GetConnector("oauth-http")
	if err != nil {
		t.Fatal(err)
	}
	if c.MCP.OAuth == nil || c.MCP.OAuth.Status != "authorized" {
		t.Fatalf("oauth after callback: %+v", c.MCP.OAuth)
	}
	if c.MCP.OAuth.TokenBundleSealed == "" || !strings.HasPrefix(c.MCP.OAuth.TokenBundleSealed, settingscrypto.Prefix) {
		t.Fatalf("token_bundle not sealed: %q", c.MCP.OAuth.TokenBundleSealed)
	}
	if c.MCP.OAuth.ClientID != "dcr-cid" {
		t.Fatalf("client_id=%q", c.MCP.OAuth.ClientID)
	}

	get := httptest.NewRequest(http.MethodGet, "/v0/connectors/oauth-http", nil)
	getRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRR, get)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d", getRR.Code)
	}
	getBody := getRR.Body.String()
	if strings.Contains(getBody, c.MCP.OAuth.TokenBundleSealed) || strings.Contains(getBody, "access-from-callback") {
		t.Fatalf("GET leaked token: %s", getBody)
	}
	if !strings.Contains(getBody, `"status":"authorized"`) {
		t.Fatalf("GET missing authorized status: %s", getBody)
	}

	// Disconnect → clear status / token_bundle (no Bearer material left).
	disc := httptest.NewRequest(http.MethodPost, "/v0/connectors/oauth-http/mcp/oauth/disconnect", nil)
	discRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(discRR, disc)
	if discRR.Code != http.StatusOK {
		t.Fatalf("disconnect status=%d body=%s", discRR.Code, discRR.Body.String())
	}
	c2, err := st.GetConnector("oauth-http")
	if err != nil {
		t.Fatal(err)
	}
	if c2.MCP.OAuth == nil {
		t.Fatal("oauth nil after disconnect")
	}
	if c2.MCP.OAuth.Status != "" || c2.MCP.OAuth.TokenBundleSealed != "" {
		t.Fatalf("after disconnect: status=%q bundle=%q", c2.MCP.OAuth.Status, c2.MCP.OAuth.TokenBundleSealed)
	}
}

func TestMCPOAuthStartRequiresClientWhenNoDCR(t *testing.T) {
	setTestSettingsKey(t)
	var asBase string
	mcpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mcp":
			w.WriteHeader(http.StatusOK)
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              asBase + "/mcp",
				"authorization_servers": []string{asBase},
			})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":                 asBase,
				"authorization_endpoint": asBase + "/authorize",
				"token_endpoint":         asBase + "/token",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mcpSrv.Close()
	asBase = mcpSrv.URL

	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID:   "no-dcr",
		Type: "mcp",
		MCP:  store.MCPConfig{Transport: "http", URL: mcpSrv.URL + "/mcp"},
	})
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.CallbackPublicBase = "http://localhost:18080"

	req := httptest.NewRequest(http.MethodPost, "/v0/connectors/no-dcr/mcp/oauth/start", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "oauth_client_required") {
		t.Fatalf("want oauth_client_required: %s", rr.Body.String())
	}
}

func TestMCPOAuthStartRequiresSettingsKey(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "")
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID:   "need-key",
		Type: "mcp",
		MCP: store.MCPConfig{
			Transport: "http",
			URL:       "http://127.0.0.1:9/mcp",
			OAuth:     &store.MCPOAuthConfig{ClientID: "pre-cid"},
		},
	})
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.CallbackPublicBase = "http://localhost:18080"

	req := httptest.NewRequest(http.MethodPost, "/v0/connectors/need-key/mcp/oauth/start", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "settings_key_required") {
		t.Fatalf("want settings_key_required: %s", rr.Body.String())
	}
}

func TestMCPOAuthCallbackClientSecretRequiresSettingsKey(t *testing.T) {
	setTestSettingsKey(t)
	key, err := settingscrypto.KeyFromEnv()
	if err != nil || len(key) == 0 {
		t.Fatalf("KeyFromEnv: %v key_len=%d", err, len(key))
	}
	sealed, err := settingscrypto.Seal(key, "client-secret-plain")
	if err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID:   "cb-secret",
		Type: "mcp",
		MCP: store.MCPConfig{
			Transport: "http",
			URL:       "http://127.0.0.1:9/mcp",
			OAuth: &store.MCPOAuthConfig{
				ClientID:           "cid",
				ClientSecretSealed: sealed,
				TokenEndpoint:      "http://127.0.0.1:9/token",
			},
		},
	})
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.OAuthSessions.Put("st-1", mcpoauth.Pending{
		ConnectorID: "cb-secret",
		Verifier:    "verifier",
		RedirectURI: "http://localhost:18080/v0/connectors/cb-secret/mcp/oauth/callback",
	})

	t.Setenv("BAIZE_SETTINGS_KEY", "")
	req := httptest.NewRequest(http.MethodGet, "/v0/connectors/cb-secret/mcp/oauth/callback?code=c1&state=st-1", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s want 400", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "settings_key_required") {
		t.Fatalf("want settings_key_required: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"internal"`) {
		t.Fatalf("must not map missing key to internal: %s", rr.Body.String())
	}
}
