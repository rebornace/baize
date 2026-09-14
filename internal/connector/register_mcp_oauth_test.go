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
