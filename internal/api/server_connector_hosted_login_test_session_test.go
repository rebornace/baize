package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestPostConnectorHostedLoginTestSession(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	srv.Identities = identity.NewMemoryStore()
	srv.CallbackPublicBase = "http://127.0.0.1:8080"
	h := srv.Handler()

	putConnectorJSON(t, h, "hl-test", map[string]any{
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

	const identityID = "mei_hl"
	if err := st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: identityID, Name: "mall-user"}); err != nil {
		t.Fatal(err)
	}
	key := store.MCPExportKey{
		ID:         "key_hl",
		Name:       "test-key",
		IdentityID: identityID,
		KeyHash:    "test-hash",
		Prefix:     "bz_test",
		IdentitySlots: []store.MCPExportIdentitySlot{{
			ConnectorID: "hl-test",
			IdentityID:  identityID,
		}},
	}
	if err := st.InsertMCPExportKey(key); err != nil {
		t.Fatal(err)
	}

	body := `{"export_key_id":"` + key.ID + `"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v0/connectors/hl-test/hosted-login/test-session", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		URL       string `json:"url"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.URL, "/v0/auth/hosted/") {
		t.Fatalf("url=%q", resp.URL)
	}
	if resp.SessionID == "" {
		t.Fatal("missing session_id")
	}
}
