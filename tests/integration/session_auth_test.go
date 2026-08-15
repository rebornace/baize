package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

const (
	sessionAuthJWTAdmin = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhZG1pbkB4LmNvbSIsImVtYWlsIjoiYWRtaW5AeC5jb20iLCJyb2xlcyI6WyJhZG1pbiJdLCJleHAiOjk5OTk5OTk5OTl9.sig"
	sessionAuthJWTUser  = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyQHguY29tIiwiZW1haWwiOiJ1c2VyQHguY29tIiwiZXhwIjo5OTk5OTk5OTk5fQ.sig"
	sessionAuthEnvToken = "ENV_FALLBACK_TOKEN"
)

// TestSessionAuthCaptureReuseDeleteForce covers design §10:
// login capture → reuse Bearer, list identities redaction, DELETE → no default-header
// fallback on conversation path, identity_id force.
func TestSessionAuthCaptureReuseDeleteForce(t *testing.T) {
	var lastAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_, _ = w.Write([]byte(`{"accessToken":"` + sessionAuthJWTAdmin + `","email":"admin@x.com"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/me":
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)

	spec := writeSessionAuthSpec(t)
	idStore := identity.NewMemoryStore()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	envAuth := "Bearer " + sessionAuthEnvToken
	_, _, err := openapi.RegisterWithOpts(st, reg, openapi.RegisterOpts{
		ID:         "session-auth",
		Type:       "openapi",
		SpecPath:   spec,
		BaseURL:    upstream.URL,
		Headers:    map[string]string{"Authorization": envAuth},
		Identities: idStore,
		Resolver:   authresolve.OpenAPISecurityResolver{},
		Capture: identity.CaptureConfig{
			ToolNameGlob:   "*login*",
			TokenJSONPaths: []string{"accessToken", "data.token"},
			LabelJSONPaths: []string{"email"},
			HeaderTemplate: "Bearer {{token}}",
			DefaultScheme:  "bearer",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	apiSrv := api.NewServer(st, reg, &sessionAuthFakeRunner{})
	apiSrv.Identities = idStore
	h := apiSrv.Handler()

	conv := "conv_session_auth"
	ctx := identity.WithConversationID(context.Background(), conv)

	// 1) Login capture → same conversation second call uses captured Bearer.
	_, isErr, err := reg.Invoke(ctx, "login", map[string]any{"email": "admin@x.com", "password": "x"})
	if err != nil || isErr {
		t.Fatalf("login: isErr=%v err=%v", isErr, err)
	}
	lastAuth = ""
	_, isErr, err = reg.Invoke(ctx, "getMe", nil)
	if err != nil || isErr {
		t.Fatalf("getMe after login: isErr=%v err=%v", isErr, err)
	}
	wantCaptured := "Bearer " + sessionAuthJWTAdmin
	if lastAuth != wantCaptured {
		t.Fatalf("after-login Authorization=%q, want %q", lastAuth, wantCaptured)
	}

	views := idStore.List(conv)
	if len(views) != 1 {
		t.Fatalf("expected 1 captured identity, got %+v", views)
	}
	capturedID := views[0].ID

	// 2) list identities API must not contain JWT plaintext.
	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/"+conv+"/identities", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list identities status=%d body=%s", rr.Code, rr.Body.String())
	}
	raw := rr.Body.String()
	if strings.Contains(raw, sessionAuthJWTAdmin) || strings.Contains(raw, sessionAuthEnvToken) {
		t.Fatalf("list identities leaked token: %s", raw)
	}
	var public []identity.PublicView
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&public); err != nil {
		t.Fatal(err)
	}
	if len(public) != 1 || public[0].ID != capturedID || public[0].Label != "admin@x.com" {
		t.Fatalf("public list=%+v", public)
	}

	// 3) DELETE identity → with conversation_id, no fallback to connector defaults.
	del := httptest.NewRequest(http.MethodDelete, "/v0/conversations/"+conv+"/identities/"+capturedID, nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, del)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete identity status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(idStore.List(conv)) != 0 {
		t.Fatalf("expected empty store after delete, got %+v", idStore.List(conv))
	}
	lastAuth = ""
	_, isErr, err = reg.Invoke(ctx, "getMe", nil)
	if err != nil || isErr {
		t.Fatalf("getMe after delete: isErr=%v err=%v", isErr, err)
	}
	if lastAuth != "" {
		t.Fatalf("after-delete Authorization=%q, want empty (conversation path skips defaults)", lastAuth)
	}

	// 4) identity_id force: ignore default, use forced identity.
	adminID, err := idStore.Upsert(conv, identity.Identity{
		Label:             "admin@x.com",
		Scheme:            "bearer",
		Subject:           "admin@x.com",
		CredentialHeaders: map[string]string{"Authorization": "Bearer " + sessionAuthJWTAdmin},
		Source:            identity.SourceLoginCapture,
		IsDefault:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := idStore.Upsert(conv, identity.Identity{
		Label:             "user@x.com",
		Scheme:            "bearer",
		Subject:           "user@x.com",
		CredentialHeaders: map[string]string{"Authorization": "Bearer " + sessionAuthJWTUser},
		Source:            identity.SourceLoginCapture,
		IsDefault:         false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if adminID == userID {
		t.Fatal("expected distinct identity ids")
	}

	lastAuth = ""
	forceCtx := identity.WithForceIdentityID(ctx, userID)
	_, isErr, err = reg.Invoke(forceCtx, "getMe", nil)
	if err != nil || isErr {
		t.Fatalf("getMe force identity: isErr=%v err=%v", isErr, err)
	}
	wantForced := "Bearer " + sessionAuthJWTUser
	if lastAuth != wantForced {
		t.Fatalf("force identity_id Authorization=%q, want %q (default was admin)", lastAuth, wantForced)
	}
}

type sessionAuthFakeRunner struct{}

func (sessionAuthFakeRunner) Execute(context.Context, string, agent.Def, string) error {
	return nil
}

func (sessionAuthFakeRunner) ContinueFromHITL(context.Context, string, run.Decision) error {
	return nil
}

func writeSessionAuthSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "session-auth.yaml")
	content := `openapi: 3.0.3
info:
  title: session-auth
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
