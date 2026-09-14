package connector_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/connector/mcpoauth"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/settingscrypto"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

const mcpOAuthTestKey = "test-settings-key-32bytes-ok!!"

type mcpOAuthEchoArgs struct {
	Message string `json:"message"`
}

type mcpOAuthEchoOutput struct {
	Message string `json:"message"`
}

type authCapture struct {
	mu   sync.Mutex
	auth []string
}

func (a *authCapture) add(v string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.auth = append(a.auth, v)
}

func (a *authCapture) last() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.auth) == 0 {
		return ""
	}
	return a.auth[len(a.auth)-1]
}

func (a *authCapture) all() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.auth))
	copy(out, a.auth)
	return out
}

func startCapturingMCPHTTP(t *testing.T) (*httptest.Server, *authCapture) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "oauth-http-mock", Version: "v0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "echo",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args mcpOAuthEchoArgs) (*mcp.CallToolResult, mcpOAuthEchoOutput, error) {
		return nil, mcpOAuthEchoOutput{Message: args.Message}, nil
	})
	inner := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	cap := &authCapture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.add(r.Header.Get("Authorization"))
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

func sealTestBundle(t *testing.T, b mcpoauth.TokenBundle) string {
	t.Helper()
	t.Setenv("BAIZE_SETTINGS_KEY", mcpOAuthTestKey)
	key, err := settingscrypto.KeyFromEnv()
	if err != nil || key == nil {
		t.Fatalf("KeyFromEnv: %v", err)
	}
	sealed, err := mcpoauth.SealBundle(key, b)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func TestMCPOAuthHTTPInvokeInjectsBearerOverStatic(t *testing.T) {
	mcpSrv, cap := startCapturingMCPHTTP(t)
	login := []string{}
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	sealed := sealTestBundle(t, mcpoauth.TokenBundle{
		AccessToken: "oauth-access",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	key, _ := settingscrypto.KeyFromEnv()

	_, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "mcp-oauth", Type: "mcp",
		SettingsKey: key,
		MCP: store.MCPConfig{
			Transport: "http",
			URL:       mcpSrv.URL,
			Headers:   map[string]string{"Authorization": "Bearer static", "X-Keep": "1"},
			OAuth: &store.MCPOAuthConfig{
				Status:            "authorized",
				ClientID:          "cid",
				TokenBundleSealed: sealed,
			},
		},
		RequireLogin: &login,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	out, isErr, invErr := reg.Invoke(context.Background(), "echo", map[string]any{"message": "hi"})
	if invErr != nil || isErr {
		t.Fatalf("invoke: isErr=%v err=%v out=%+v", isErr, invErr, out)
	}
	if out["message"] != "hi" {
		t.Fatalf("echo=%+v", out)
	}
	if got := cap.last(); got != "Bearer oauth-access" {
		t.Fatalf("Authorization=%q want Bearer oauth-access; all=%v", got, cap.all())
	}
}

func TestMCPOAuthHTTPInvokeRefreshesAndWritesBack(t *testing.T) {
	mcpSrv, cap := startCapturingMCPHTTP(t)
	const freshAccess = "fresh-access-token"
	var refreshHits int
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshHits++
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("grant_type") != "refresh_token" || form.Get("refresh_token") != "rt-old" {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  freshAccess,
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "rt-new",
		})
	}))
	t.Cleanup(tokenSrv.Close)

	login := []string{}
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	sealed := sealTestBundle(t, mcpoauth.TokenBundle{
		AccessToken:  "old-access",
		RefreshToken: "rt-old",
		ExpiresAt:    time.Now().Add(-time.Minute),
	})
	key, _ := settingscrypto.KeyFromEnv()

	_, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "mcp-refresh", Type: "mcp",
		SettingsKey: key,
		MCP: store.MCPConfig{
			Transport: "http",
			URL:       mcpSrv.URL,
			OAuth: &store.MCPOAuthConfig{
				Status:            "authorized",
				ClientID:          "cid",
				TokenEndpoint:     tokenSrv.URL,
				TokenBundleSealed: sealed,
			},
		},
		RequireLogin: &login,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if refreshHits == 0 {
		t.Fatal("expected refresh during Apply discover")
	}

	out, isErr, invErr := reg.Invoke(context.Background(), "echo", map[string]any{"message": "ok"})
	if invErr != nil || isErr {
		t.Fatalf("invoke: isErr=%v err=%v out=%+v", isErr, invErr, out)
	}
	if got := cap.last(); got != "Bearer "+freshAccess {
		t.Fatalf("Authorization=%q want Bearer %s", got, freshAccess)
	}

	c, err := st.GetConnector("mcp-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if c.MCP.OAuth == nil || c.MCP.OAuth.TokenBundleSealed == "" || c.MCP.OAuth.TokenBundleSealed == sealed {
		t.Fatalf("expected refreshed sealed bundle written back, got %+v", c.MCP.OAuth)
	}
	opened, err := mcpoauth.OpenBundle(key, c.MCP.OAuth.TokenBundleSealed)
	if err != nil {
		t.Fatal(err)
	}
	if opened.AccessToken != freshAccess {
		t.Fatalf("stored access=%q", opened.AccessToken)
	}
}

func TestMCPOAuthHTTPInvokeNeedsReauth(t *testing.T) {
	mcpSrv, _ := startCapturingMCPHTTP(t)
	login := []string{}
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	// Valid token so Apply/discover succeeds.
	valid := sealTestBundle(t, mcpoauth.TokenBundle{
		AccessToken: "valid-for-discover",
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	key, _ := settingscrypto.KeyFromEnv()
	_, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "mcp-reauth", Type: "mcp",
		SettingsKey: key,
		MCP: store.MCPConfig{
			Transport: "http",
			URL:       mcpSrv.URL,
			OAuth: &store.MCPOAuthConfig{
				Status:            "authorized",
				ClientID:          "cid",
				TokenBundleSealed: valid,
			},
		},
		RequireLogin: &login,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	expired := sealTestBundle(t, mcpoauth.TokenBundle{
		AccessToken: "expired-only",
		ExpiresAt:   time.Now().Add(-time.Minute),
	})
	c, _ := st.GetConnector("mcp-reauth")
	c.MCP.OAuth.TokenBundleSealed = expired
	c.MCP.OAuth.Status = "authorized"
	st.UpsertConnector(c)

	out, isErr, invErr := reg.Invoke(context.Background(), "echo", map[string]any{"message": "x"})
	if invErr != nil {
		t.Fatalf("invoke err=%v (want tool IsError content)", invErr)
	}
	if !isErr {
		t.Fatalf("want IsError=true, out=%+v", out)
	}
	if out["code"] != "oauth_reauth_required" {
		t.Fatalf("code=%v out=%+v", out["code"], out)
	}
	errMsg, _ := out["error"].(string)
	if !strings.Contains(errMsg, "重新授权") {
		t.Fatalf("error message=%q", errMsg)
	}

	c2, _ := st.GetConnector("mcp-reauth")
	if c2.MCP.OAuth == nil || c2.MCP.OAuth.Status != "needs_reauth" {
		t.Fatalf("status=%v want needs_reauth", c2.MCP.OAuth)
	}
}

func TestMCPOAuthHTTPInvokeRejectsMissingSettingsKey(t *testing.T) {
	mcpSrv, cap := startCapturingMCPHTTP(t)
	login := []string{}
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	// Seal with key present, then clear env so Apply/invoke see no settings key.
	sealed := sealTestBundle(t, mcpoauth.TokenBundle{
		AccessToken: "oauth-access",
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	t.Setenv("BAIZE_SETTINGS_KEY", "")

	_, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "mcp-nokey", Type: "mcp",
		SettingsKey: nil,
		MCP: store.MCPConfig{
			Transport: "http",
			URL:       mcpSrv.URL,
		},
		RequireLogin: &login,
	})
	if err != nil {
		t.Fatalf("Apply without oauth: %v", err)
	}

	c, err := st.GetConnector("mcp-nokey")
	if err != nil {
		t.Fatal(err)
	}
	c.MCP.OAuth = &store.MCPOAuthConfig{
		Status:            "authorized",
		ClientID:          "cid",
		TokenBundleSealed: sealed,
	}
	st.UpsertConnector(c)

	before := len(cap.all())
	out, isErr, invErr := reg.Invoke(context.Background(), "echo", map[string]any{"message": "x"})
	if invErr != nil {
		t.Fatalf("invoke err=%v (want tool IsError content)", invErr)
	}
	if !isErr {
		t.Fatalf("want IsError=true, out=%+v", out)
	}
	if out["code"] != "settings_key_required" {
		t.Fatalf("code=%v out=%+v", out["code"], out)
	}
	for _, a := range cap.all()[before:] {
		if strings.HasPrefix(a, "Bearer ") {
			t.Fatalf("must not send Bearer without settings key; got %q", a)
		}
	}
}

func TestMCPOAuthHTTPInvokeAfterDisconnectHasNoBearer(t *testing.T) {
	mcpSrv, cap := startCapturingMCPHTTP(t)
	login := []string{}
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	sealed := sealTestBundle(t, mcpoauth.TokenBundle{
		AccessToken: "oauth-access",
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	key, _ := settingscrypto.KeyFromEnv()
	_, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "mcp-disc", Type: "mcp",
		SettingsKey: key,
		MCP: store.MCPConfig{
			Transport: "http",
			URL:       mcpSrv.URL,
			OAuth: &store.MCPOAuthConfig{
				Status:            "authorized",
				ClientID:          "cid",
				TokenBundleSealed: sealed,
			},
		},
		RequireLogin: &login,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	_, isErr, invErr := reg.Invoke(context.Background(), "echo", map[string]any{"message": "a"})
	if invErr != nil || isErr {
		t.Fatalf("invoke before disconnect: isErr=%v err=%v", isErr, invErr)
	}
	if cap.last() != "Bearer oauth-access" {
		t.Fatalf("before disconnect Authorization=%q", cap.last())
	}

	// Simulate task-5 disconnect: clear bundle/status in store without re-Apply.
	c, _ := st.GetConnector("mcp-disc")
	c.MCP.OAuth.Status = ""
	c.MCP.OAuth.TokenBundleSealed = ""
	st.UpsertConnector(c)

	before := len(cap.all())
	_, isErr, invErr = reg.Invoke(context.Background(), "echo", map[string]any{"message": "b"})
	if invErr != nil || isErr {
		t.Fatalf("invoke after disconnect: isErr=%v err=%v", isErr, invErr)
	}
	after := cap.all()[before:]
	for _, a := range after {
		if strings.HasPrefix(a, "Bearer ") {
			t.Fatalf("after disconnect still sent Authorization=%q; all after=%v", a, after)
		}
	}
}

func TestApplySoftFailsOAuthReauthKeepsExistingTools(t *testing.T) {
	mcpSrv, _ := startCapturingMCPHTTP(t)
	login := []string{}
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	valid := sealTestBundle(t, mcpoauth.TokenBundle{
		AccessToken: "valid-for-discover",
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	key, _ := settingscrypto.KeyFromEnv()
	in := connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "mcp-soft", Type: "mcp",
		SettingsKey: key,
		MCP: store.MCPConfig{
			Transport: "http",
			URL:       mcpSrv.URL,
			Headers:   map[string]string{"X-Edit": "before"},
			OAuth: &store.MCPOAuthConfig{
				Status:            "authorized",
				ClientID:          "cid",
				TokenBundleSealed: valid,
			},
		},
		RequireLogin: &login,
	}
	if _, _, err := connector.Apply(in); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	toolsBefore := st.ListToolsByConnector("mcp-soft")
	if len(toolsBefore) == 0 {
		t.Fatal("expected discovered tools after first Apply")
	}

	expired := sealTestBundle(t, mcpoauth.TokenBundle{
		AccessToken: "expired-only",
		ExpiresAt:   time.Now().Add(-time.Minute),
	})
	in.MCP.Headers = map[string]string{"X-Edit": "after"}
	in.MCP.OAuth = &store.MCPOAuthConfig{
		Status:            "authorized",
		ClientID:          "cid",
		TokenBundleSealed: expired,
	}
	conn, _, err := connector.Apply(in)
	if err != nil {
		t.Fatalf("Apply with oauth_reauth must soft-fail (save headers/URL): %v", err)
	}
	if conn.MCP.Headers["X-Edit"] != "after" {
		t.Fatalf("headers not saved: %+v", conn.MCP.Headers)
	}
	if conn.MCP.OAuth == nil || conn.MCP.OAuth.Status != "needs_reauth" {
		t.Fatalf("oauth status=%v want needs_reauth", conn.MCP.OAuth)
	}
	toolsAfter := st.ListToolsByConnector("mcp-soft")
	if len(toolsAfter) != len(toolsBefore) {
		t.Fatalf("tools after soft-fail=%d want keep %d", len(toolsAfter), len(toolsBefore))
	}
}

func TestApplySoftFailsSettingsKeyRequiredKeepsExistingTools(t *testing.T) {
	mcpSrv, _ := startCapturingMCPHTTP(t)
	login := []string{}
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	sealed := sealTestBundle(t, mcpoauth.TokenBundle{
		AccessToken: "oauth-access",
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	key, _ := settingscrypto.KeyFromEnv()
	if _, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "mcp-soft-key", Type: "mcp",
		SettingsKey: key,
		MCP: store.MCPConfig{
			Transport: "http",
			URL:       mcpSrv.URL,
			OAuth: &store.MCPOAuthConfig{
				Status:            "authorized",
				ClientID:          "cid",
				TokenBundleSealed: sealed,
			},
		},
		RequireLogin: &login,
	}); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	before := len(st.ListToolsByConnector("mcp-soft-key"))
	if before == 0 {
		t.Fatal("expected tools")
	}

	t.Setenv("BAIZE_SETTINGS_KEY", "")
	conn, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "mcp-soft-key", Type: "mcp",
		SettingsKey: nil,
		MCP: store.MCPConfig{
			Transport: "http",
			URL:       mcpSrv.URL + "/edited",
			Headers:   map[string]string{"X-Keep": "1"},
			OAuth: &store.MCPOAuthConfig{
				Status:            "authorized",
				ClientID:          "cid",
				TokenBundleSealed: sealed,
			},
		},
		RequireLogin: &login,
	})
	if err != nil {
		t.Fatalf("Apply without settings key must soft-fail: %v", err)
	}
	if conn.MCP.URL != mcpSrv.URL+"/edited" {
		t.Fatalf("URL not saved: %q", conn.MCP.URL)
	}
	if got := len(st.ListToolsByConnector("mcp-soft-key")); got != before {
		t.Fatalf("tools=%d want keep %d", got, before)
	}
}

func TestMCPOAuthHTTPInvokeTransientRefreshDoesNotNeedsReauth(t *testing.T) {
	mcpSrv, _ := startCapturingMCPHTTP(t)
	login := []string{}
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	valid := sealTestBundle(t, mcpoauth.TokenBundle{
		AccessToken: "valid-for-discover",
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	key, _ := settingscrypto.KeyFromEnv()
	if _, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "mcp-transient", Type: "mcp",
		SettingsKey: key,
		MCP: store.MCPConfig{
			Transport: "http",
			URL:       mcpSrv.URL,
			OAuth: &store.MCPOAuthConfig{
				Status:            "authorized",
				ClientID:          "cid",
				TokenEndpoint:     "http://127.0.0.1:1/token",
				TokenBundleSealed: valid,
			},
		},
		RequireLogin: &login,
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	expired := sealTestBundle(t, mcpoauth.TokenBundle{
		AccessToken:  "old",
		RefreshToken: "rt",
		ExpiresAt:    time.Now().Add(-time.Minute),
	})
	c, _ := st.GetConnector("mcp-transient")
	c.MCP.OAuth.TokenBundleSealed = expired
	c.MCP.OAuth.Status = "authorized"
	c.MCP.OAuth.TokenEndpoint = "http://127.0.0.1:1/token"
	st.UpsertConnector(c)

	out, isErr, invErr := reg.Invoke(context.Background(), "echo", map[string]any{"message": "x"})
	if invErr == nil {
		t.Fatalf("want transient invoke error, out=%+v isErr=%v", out, isErr)
	}
	if !strings.Contains(invErr.Error(), "暂时不可用") {
		t.Fatalf("want readable transient error, got %v", invErr)
	}
	if out != nil {
		if code, _ := out["code"].(string); code == "oauth_reauth_required" {
			t.Fatalf("transient must not return oauth_reauth_required: %+v", out)
		}
	}
	c2, _ := st.GetConnector("mcp-transient")
	if c2.MCP.OAuth.Status != "authorized" {
		t.Fatalf("status=%q must stay authorized on transient", c2.MCP.OAuth.Status)
	}
}
