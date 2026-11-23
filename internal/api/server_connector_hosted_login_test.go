package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// PUT with hosted_login, then PUT omitting it, must keep the config.
func TestPutConnectorOmitsHostedLoginPreservesExisting(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	srv.Identities = identity.NewMemoryStore()
	h := srv.Handler()

	putConnectorJSON(t, h, "hl1", map[string]any{
		"type":     "openapi",
		"spec":     writeLoginGetMeAPISpec(t),
		"base_url": upstream.URL,
		"auth": map[string]any{
			"hosted_login": map[string]any{
				"enabled":           true,
				"default_method_id": "default",
				"methods": []map[string]any{{
					"id": "default", "login_tool": "login", "mode": hostedlogin.ModePasswordForm,
					"fields": []map[string]any{
						{"name": "user", "arg": "username", "input": "text"},
						{"name": "pass", "arg": "password", "input": "password"},
					},
				}},
			},
		},
	})

	putConnectorJSON(t, h, "hl1", map[string]any{
		"type":     "openapi",
		"base_url": upstream.URL,
	})

	got, err := st.GetConnector("hl1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	hl := got.Auth.HostedLogin
	method, ok := hl.DefaultMethod()
	if !ok || !hl.Enabled || method.LoginTool != "login" || method.Mode != hostedlogin.ModePasswordForm {
		t.Fatalf("hosted_login not preserved: %+v", hl)
	}
	if len(method.Fields) != 2 {
		t.Fatalf("fields not preserved: %+v", method.Fields)
	}

	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, httptest.NewRequest(http.MethodGet, "/v0/connectors/hl1", nil))
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	body := getRR.Body.String()
	for _, part := range []string{`"hosted_login"`, `"login_tool":"login"`, `"enabled":true`, `"methods"`} {
		if !strings.Contains(body, part) {
			t.Fatalf("GET must contain %s, got %s", part, body)
		}
	}
}

func TestPutConnectorExplicitHostedLoginOverrides(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	h := srv.Handler()

	putConnectorJSON(t, h, "hl2", map[string]any{
		"type":     "openapi",
		"spec":     writeLoginGetMeAPISpec(t),
		"base_url": upstream.URL,
		"auth": map[string]any{
			"hosted_login": map[string]any{
				"enabled":           true,
				"default_method_id": "default",
				"methods": []map[string]any{{
					"id": "default", "login_tool": "login", "mode": hostedlogin.ModePasswordForm,
					"fields": []map[string]any{
						{"name": "user", "arg": "username", "input": "text"},
						{"name": "pass", "arg": "password", "input": "password"},
					},
				}},
			},
		},
	})
	putConnectorJSON(t, h, "hl2", map[string]any{
		"type":     "openapi",
		"base_url": upstream.URL,
		"auth": map[string]any{
			"hosted_login": map[string]any{
				"enabled": false, "methods": []any{},
			},
		},
	})
	got, _ := st.GetConnector("hl2")
	if got.Auth.HostedLogin.Enabled {
		t.Fatalf("explicit clear not honored: %+v", got.Auth.HostedLogin)
	}
	if m, ok := got.Auth.HostedLogin.DefaultMethod(); ok && m.LoginTool != "" {
		t.Fatalf("explicit clear not honored: %+v", got.Auth.HostedLogin)
	}
}
