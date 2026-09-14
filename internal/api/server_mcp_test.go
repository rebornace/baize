package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func mcpAPIRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func buildMCPMockBinaryAPI(t *testing.T) string {
	t.Helper()
	root := mcpAPIRepoRoot(t)
	name := "mcp-mock"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(root, "bin", name)
	cmd := exec.Command("go", "build", "-o", out, filepath.Join(root, "examples", "mcp-mock"))
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build mcp-mock: %v\n%s", err, output)
	}
	return out
}

func TestPutMCPStdioConnectorRegistersTools(t *testing.T) {
	t.Setenv("MCP_API_SECRET", "super-secret")
	mock := buildMCPMockBinaryAPI(t)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/analytics",
		jsonBody(t, map[string]any{
			"type": "mcp",
			"mcp": map[string]any{
				"transport": "stdio",
				"command":   mock,
				"env": map[string]string{
					"TOKEN": "env:MCP_API_SECRET",
				},
			},
			"auth": map[string]any{
				"mode": "static",
				"static": map[string]any{
					"headers": map[string]string{"Authorization": "Bearer ignored"},
				},
			},
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", rr.Code, rr.Body.String())
	}
	putBody := rr.Body.String()
	if strings.Contains(putBody, "super-secret") {
		t.Fatalf("PUT leaked resolved env secret: %s", putBody)
	}
	if !strings.Contains(putBody, "env:MCP_API_SECRET") {
		t.Fatalf("PUT missing env ref shape: %s", putBody)
	}

	var put struct {
		ID    string          `json:"id"`
		Type  string          `json:"type"`
		MCP   store.MCPConfig `json:"mcp"`
		Tools []tool.Info     `json:"tools"`
	}
	if err := json.NewDecoder(strings.NewReader(putBody)).Decode(&put); err != nil {
		t.Fatal(err)
	}
	if put.ID != "analytics" || put.Type != "mcp" {
		t.Fatalf("put body=%+v", put)
	}
	if put.MCP.Transport != "stdio" || put.MCP.Command != mock {
		t.Fatalf("mcp=%+v", put.MCP)
	}
	if put.MCP.Env["TOKEN"] != "env:MCP_API_SECRET" {
		t.Fatalf("env=%+v", put.MCP.Env)
	}
	echoFound := false
	for _, info := range put.Tools {
		if info.Name == "echo" {
			echoFound = true
		}
	}
	if !echoFound {
		t.Fatalf("tools=%+v", put.Tools)
	}
	if len(reg.List()) == 0 {
		t.Fatal("registry empty after PUT")
	}

	get := httptest.NewRequest(http.MethodGet, "/v0/connectors/analytics", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", rr.Code, rr.Body.String())
	}
	getBody := rr.Body.String()
	if strings.Contains(getBody, "super-secret") {
		t.Fatalf("GET leaked resolved env secret: %s", getBody)
	}
	if !strings.Contains(getBody, "env:MCP_API_SECRET") {
		t.Fatalf("GET missing env ref shape: %s", getBody)
	}
	var got struct {
		Type string              `json:"type"`
		MCP  store.MCPConfig     `json:"mcp"`
		Auth store.ConnectorAuth `json:"auth"`
	}
	if err := json.NewDecoder(strings.NewReader(getBody)).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Type != "mcp" {
		t.Fatalf("type=%q", got.Type)
	}
	if got.MCP.Command != mock || got.MCP.Env["TOKEN"] != "env:MCP_API_SECRET" {
		t.Fatalf("mcp=%+v", got.MCP)
	}
	if got.Auth.Mode != "" || len(got.Auth.Static.Headers) != 0 {
		t.Fatalf("auth should be ignored for mcp, got %+v", got.Auth)
	}
}

func TestListConnectorsTypeMCPIncludesEmptyTools(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID:   "gugu",
		Type: "mcp",
		MCP: store.MCPConfig{
			Transport: "http",
			URL:       "https://mcp.gugudata.com/mcp",
		},
	})
	st.UpsertConnector(store.Connector{
		ID:      "openapi1",
		Type:    "openapi",
		Spec:    "x.yaml",
		BaseURL: "http://example",
	})
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	req := httptest.NewRequest(http.MethodGet, "/v0/connectors?type=mcp", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Connectors []struct {
			ID    string `json:"id"`
			Type  string `json:"type"`
			Tools []any  `json:"tools"`
			MCP   struct {
				URL string `json:"url"`
			} `json:"mcp"`
		} `json:"connectors"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Connectors) != 1 || got.Connectors[0].ID != "gugu" {
		t.Fatalf("want only gugu mcp, got %+v", got.Connectors)
	}
	if got.Connectors[0].MCP.URL != "https://mcp.gugudata.com/mcp" {
		t.Fatalf("mcp.url=%q", got.Connectors[0].MCP.URL)
	}
	if got.Connectors[0].Tools == nil {
		t.Fatal("tools must be [] not null")
	}
}

func TestGetConnectorMCPOAuthRedacted(t *testing.T) {
	const (
		tokenBundle  = "bz1:fake-token-bundle-sealed"
		clientSecret = "bz1:fake-client-secret-sealed"
	)
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID:   "oauth-mcp",
		Type: "mcp",
		MCP: store.MCPConfig{
			Transport: "stdio",
			Command:   "noop",
			OAuth: &store.MCPOAuthConfig{
				Status:               "authorized",
				ClientID:             "my-client-id",
				ClientSecretSealed:   clientSecret,
				TokenBundleSealed:    tokenBundle,
				AuthorizationEndpoint: "https://auth.example/authorize",
				TokenEndpoint:         "https://auth.example/token",
			},
		},
	})
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	req := httptest.NewRequest(http.MethodGet, "/v0/connectors/oauth-mcp", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, tokenBundle) || strings.Contains(body, clientSecret) {
		t.Fatalf("GET leaked oauth secrets: %s", body)
	}
	var got struct {
		MCP struct {
			OAuth struct {
				Status   string `json:"status"`
				ClientID string `json:"client_id"`
			} `json:"oauth"`
		} `json:"mcp"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.MCP.OAuth.Status != "authorized" || got.MCP.OAuth.ClientID != "my-client-id" {
		t.Fatalf("oauth public fields missing: %+v", got.MCP.OAuth)
	}
}

func TestPutConnectorMCPOAuthPreservesEmptySecrets(t *testing.T) {
	mock := buildMCPMockBinaryAPI(t)
	const (
		tokenBundle  = "bz1:stored-token-bundle"
		clientSecret = "bz1:stored-client-secret"
	)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	put1 := httptest.NewRequest(http.MethodPut, "/v0/connectors/oauth-preserve",
		jsonBody(t, map[string]any{
			"type": "mcp",
			"mcp": map[string]any{
				"transport": "stdio",
				"command":   mock,
				"oauth": map[string]any{
					"status":        "authorized",
					"client_id":     "cid-1",
					"client_secret": clientSecret,
					"token_bundle":  tokenBundle,
				},
			},
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put1)
	if rr.Code != http.StatusOK {
		t.Fatalf("initial PUT status=%d body=%s", rr.Code, rr.Body.String())
	}

	put2 := httptest.NewRequest(http.MethodPut, "/v0/connectors/oauth-preserve",
		jsonBody(t, map[string]any{
			"type": "mcp",
			"mcp": map[string]any{
				"transport": "stdio",
				"command":   mock,
				"oauth": map[string]any{
					"status":    "needs_reauth",
					"client_id": "cid-1",
				},
			},
		}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put2)
	if rr.Code != http.StatusOK {
		t.Fatalf("second PUT status=%d body=%s", rr.Code, rr.Body.String())
	}

	c, err := st.GetConnector("oauth-preserve")
	if err != nil {
		t.Fatal(err)
	}
	if c.MCP.OAuth == nil {
		t.Fatal("oauth nil after PUT")
	}
	if c.MCP.OAuth.TokenBundleSealed != tokenBundle {
		t.Fatalf("token_bundle=%q want %q", c.MCP.OAuth.TokenBundleSealed, tokenBundle)
	}
	if c.MCP.OAuth.ClientSecretSealed != clientSecret {
		t.Fatalf("client_secret=%q want %q", c.MCP.OAuth.ClientSecretSealed, clientSecret)
	}
	if c.MCP.OAuth.Status != "needs_reauth" {
		t.Fatalf("status=%q", c.MCP.OAuth.Status)
	}
}

func TestPutMCPInvalidConfig(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})

	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/bad-mcp",
		jsonBody(t, map[string]any{
			"type": "mcp",
			"mcp": map[string]any{
				"transport": "stdio",
				"command":   filepath.Join(t.TempDir(), "nonexistent-mcp-binary"),
			},
		}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "invalid_mcp" {
		t.Fatalf("code=%q body=%s", wrap.Error.Code, rr.Body.String())
	}
	if len(reg.List()) != 0 {
		t.Fatalf("registry polluted: %+v", reg.List())
	}
}
