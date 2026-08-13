package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

const (
	authModesEnvToken    = "AUTH_MODES_ENV_TOKEN"
	authModesCapturedJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJjYXB0dXJlZEB4LmNvbSIsImV4cCI6OTk5OTk5OTk5OX0.sig"
)

// TestConnectorAuthModesStatic verifies static mode: ${ENV} is expanded at
// registration and used as DefaultHeaders when no session identity exists.
func TestConnectorAuthModesStatic(t *testing.T) {
	t.Setenv("AUTH_MODES_STATIC_TOK", "static-secret")
	var lastAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)

	spec := writeAuthModesProbeSpec(t, "probeStatic", "/probe-static")
	st := store.NewMemory()
	reg := tool.NewRegistry()
	headers, err := authcred.ResolveDefaults(authcred.Config{
		Mode: authcred.ModeStatic,
		Static: authcred.Static{Headers: map[string]string{
			"Authorization": "Bearer ${AUTH_MODES_STATIC_TOK}",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := openapi.RegisterWithOpts(st, reg, openapi.RegisterOpts{
		ID:       "auth-static",
		SpecPath: spec,
		BaseURL:  upstream.URL,
		Headers:  headers,
		AuthMode: authcred.ModeStatic,
	}); err != nil {
		t.Fatal(err)
	}
	_, isErr, invErr := reg.Invoke(context.Background(), "probeStatic", map[string]any{})
	if invErr != nil || isErr {
		t.Fatalf("invoke: isErr=%v err=%v", isErr, invErr)
	}
	if want := "Bearer static-secret"; lastAuth != want {
		t.Fatalf("Authorization=%q want %q", lastAuth, want)
	}
}

// TestConnectorAuthModesVaultRefFile verifies vault_ref file: reference is
// resolved at registration and used as DefaultHeaders.
func TestConnectorAuthModesVaultRefFile(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "tok")
	if err := os.WriteFile(tokenPath, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var lastAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)

	spec := writeAuthModesProbeSpec(t, "probeVault", "/probe-vault")
	st := store.NewMemory()
	reg := tool.NewRegistry()
	headers, err := authcred.ResolveDefaults(authcred.Config{
		Mode: authcred.ModeVaultRef,
		VaultRef: authcred.VaultRef{Headers: map[string]string{
			"Authorization": "file:" + tokenPath,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := openapi.RegisterWithOpts(st, reg, openapi.RegisterOpts{
		ID:       "auth-vault",
		SpecPath: spec,
		BaseURL:  upstream.URL,
		Headers:  headers,
		AuthMode: authcred.ModeVaultRef,
	}); err != nil {
		t.Fatal(err)
	}
	_, isErr, invErr := reg.Invoke(context.Background(), "probeVault", map[string]any{})
	if invErr != nil || isErr {
		t.Fatalf("invoke: isErr=%v err=%v", isErr, invErr)
	}
	if want := "file-secret"; lastAuth != want {
		t.Fatalf("Authorization=%q want %q", lastAuth, want)
	}
}

// TestConnectorAuthModesPassthrough verifies passthrough mode end-to-end:
// POST /v0/runs carries Authorization; the run.Engine injects it into the
// tool invoke; the downstream upstream sees it; GET /v0/runs/{id} and the
// events JSON must not contain the token.
func TestConnectorAuthModesPassthrough(t *testing.T) {
	const token = "Bearer PASSTHROUGH_SECRET"
	var (
		lastAuthMu sync.Mutex
		lastAuth   string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuthMu.Lock()
		lastAuth = r.Header.Get("Authorization")
		lastAuthMu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)

	spec := writeAuthModesProbeSpec(t, "probePass", "/probe-pass")
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "agent-pass", System: "s"})
	reg := tool.NewRegistry()
	if _, _, err := openapi.RegisterWithOpts(st, reg, openapi.RegisterOpts{
		ID:       "auth-pass",
		SpecPath: spec,
		BaseURL:  upstream.URL,
		AuthMode: authcred.ModePassthrough,
	}); err != nil {
		t.Fatal(err)
	}

	engine := &run.Engine{
		Store:    st,
		LLM:      &authModesScriptLLM{toolName: "probePass"},
		Tools:    reg,
		Gate:     run.NewGate(),
		MaxSteps: 4,
	}
	apiSrv := api.NewServer(st, reg, engine)
	apiSrv.AuthMode = authcred.ModePassthrough
	h := apiSrv.Handler()

	postReq := httptest.NewRequest(http.MethodPost, "/v0/runs",
		strings.NewReader(`{"agent_id":"agent-pass","input":"go"}`))
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Authorization", token)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, postReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /v0/runs status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		RunID string `json:"run_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.RunID == "" {
		t.Fatalf("missing run_id: %s", rr.Body.String())
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		lastAuthMu.Lock()
		got := lastAuth
		lastAuthMu.Unlock()
		if got != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	lastAuthMu.Lock()
	got := lastAuth
	lastAuthMu.Unlock()
	if got != token {
		t.Fatalf("downstream Authorization=%q want %q", got, token)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v0/runs/"+created.RunID, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET run status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "PASSTHROUGH_SECRET") {
		t.Fatalf("GET /v0/runs/{id} leaked token: %s", rr.Body.String())
	}

	evReq := httptest.NewRequest(http.MethodGet, "/v0/runs/"+created.RunID+"/events", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, evReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET events status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "PASSTHROUGH_SECRET") {
		t.Fatalf("events leaked token: %s", rr.Body.String())
	}
}

// authModesScriptLLM is a deterministic LLM that calls one GET tool then ends.
type authModesScriptLLM struct {
	toolName string
	calls    int
}

func (s *authModesScriptLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	s.calls++
	if s.calls == 1 {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "call_probe", Name: s.toolName, Arguments: map[string]any{}},
		}}, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: "done"}, nil
}

// TestConnectorAuthModesIdentityPriority verifies that a captured session
// identity takes precedence over vault_ref default headers resolved from ENV.
func TestConnectorAuthModesIdentityPriority(t *testing.T) {
	t.Setenv("AUTH_MODES_ENV_TOK", authModesEnvToken)
	var lastAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_, _ = w.Write([]byte(`{"accessToken":"` + authModesCapturedJWT + `","email":"captured@x.com"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/me":
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)

	spec := writeAuthModesLoginGetMeSpec(t)
	idStore := identity.NewMemoryStore()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	headers, err := authcred.ResolveDefaults(authcred.Config{
		Mode: authcred.ModeVaultRef,
		VaultRef: authcred.VaultRef{Headers: map[string]string{
			"Authorization": "env:AUTH_MODES_ENV_TOK",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := openapi.RegisterWithOpts(st, reg, openapi.RegisterOpts{
		ID:         "auth-priority",
		SpecPath:   spec,
		BaseURL:    upstream.URL,
		Headers:    headers,
		AuthMode:   authcred.ModeVaultRef,
		Identities: idStore,
		Resolver:   authresolve.OpenAPISecurityResolver{},
		Capture: identity.CaptureConfig{
			ToolNameGlob:   "*login*",
			TokenJSONPaths: []string{"accessToken", "data.token"},
			LabelJSONPaths: []string{"email"},
			HeaderTemplate: "Bearer {{token}}",
			DefaultScheme:  "bearer",
		},
	}); err != nil {
		t.Fatal(err)
	}

	conv := "conv_auth_modes_priority"
	ctx := identity.WithConversationID(context.Background(), conv)

	if _, isErr, err := reg.Invoke(ctx, "login", map[string]any{"email": "captured@x.com", "password": "x"}); err != nil || isErr {
		t.Fatalf("login: isErr=%v err=%v", isErr, err)
	}
	views := idStore.List(conv)
	if len(views) != 1 {
		t.Fatalf("expected 1 captured identity, got %+v", views)
	}

	lastAuth = ""
	if _, isErr, err := reg.Invoke(ctx, "getMe", nil); err != nil || isErr {
		t.Fatalf("getMe after capture: isErr=%v err=%v", isErr, err)
	}
	wantCaptured := "Bearer " + authModesCapturedJWT
	if lastAuth != wantCaptured {
		t.Fatalf("after-capture Authorization=%q want %q (captured, not ENV)", lastAuth, wantCaptured)
	}
	if lastAuth == authModesEnvToken {
		t.Fatalf("downstream used ENV token instead of captured identity")
	}
}

func writeAuthModesProbeSpec(t *testing.T, opID, path string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "probe.yaml")
	content := "openapi: 3.0.3\n" +
		"info:\n  title: probe\n  version: 0.1.0\n" +
		"paths:\n  " + path + ":\n    get:\n      operationId: " + opID + "\n" +
		"      responses:\n        \"200\":\n          description: ok\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeAuthModesLoginGetMeSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "auth-modes.yaml")
	content := `openapi: 3.0.3
info:
  title: auth-modes
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
