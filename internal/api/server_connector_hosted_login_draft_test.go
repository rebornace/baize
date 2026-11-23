package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestPostConnectorHostedLoginDraft(t *testing.T) {
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

	putConnectorJSON(t, h, "hl-draft", map[string]any{
		"type":     "openapi",
		"spec":     writeLoginGetMeAPISpec(t),
		"base_url": upstream.URL,
	})

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/connectors/hl-draft/hosted-login/draft", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		HostedLogin store.HostedLoginConfig `json:"hosted_login"`
		Source      string                  `json:"source"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.HostedLogin.Enabled {
		t.Fatal("draft must not be enabled")
	}
	if body.Source != "login_tool" {
		t.Fatalf("source=%q config=%+v", body.Source, body.HostedLogin)
	}
	m, ok := body.HostedLogin.DefaultMethod()
	if !ok || m.LoginTool == "" || m.Mode == "" {
		t.Fatalf("expected inferred method, got %+v", body.HostedLogin)
	}
}
