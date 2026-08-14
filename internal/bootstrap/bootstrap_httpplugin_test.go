package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestRegisterConnectorHTTPNoSpec(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"echo"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer sidecar.Close()

	st := store.NewMemory()
	reg := tool.NewRegistry()
	cfg := config.Config{}
	cfg.Connector.ID = "side"
	cfg.Connector.Type = "http"
	cfg.Connector.BaseURL = sidecar.URL
	// Spec intentionally empty for http.

	if err := registerConnector(st, reg, cfg, identity.NewMemoryStore()); err != nil {
		t.Fatalf("registerConnector: %v", err)
	}
	c, err := st.GetConnector("side")
	if err != nil || c.Type != "http" || c.Spec != "" {
		t.Fatalf("store %+v err=%v", c, err)
	}
	if len(reg.List()) != 1 || reg.List()[0].Name != "echo" {
		t.Fatalf("registry=%+v", reg.List())
	}
}

func TestRegisterConnectorHTTPMissingBaseURL(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	cfg := config.Config{}
	cfg.Connector.ID = "side"
	cfg.Connector.Type = "http"

	err := registerConnector(st, reg, cfg, identity.NewMemoryStore())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "connector.base_url is required") {
		t.Fatalf("err=%v", err)
	}
}

func TestRegisterConnectorHTTPSessionIdentityHeadersPreferStatic(t *testing.T) {
	var lastAuth string
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"echo"}]}`))
		case strings.HasSuffix(r.URL.Path, "/invoke"):
			_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer sidecar.Close()

	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	cfg := config.Config{}
	cfg.Connector.ID = "side"
	cfg.Connector.Type = "http"
	cfg.Connector.BaseURL = sidecar.URL
	cfg.Connector.Auth.Mode = "static"
	cfg.Connector.Auth.Static.Headers = map[string]string{
		"Authorization": "Bearer STATIC",
	}

	if err := registerConnector(st, reg, cfg, ids); err != nil {
		t.Fatalf("registerConnector: %v", err)
	}

	const conv = "conv_http_id"
	if _, err := ids.Upsert(conv, identity.Identity{
		Label:             "session",
		Scheme:            "bearer",
		Source:            identity.SourceLoginCapture,
		IsDefault:         true,
		CredentialHeaders: map[string]string{"Authorization": "Bearer SESSION"},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	ctx := identity.WithConversationID(context.Background(), conv)
	content, isErr, invErr := reg.Invoke(ctx, "echo", map[string]any{})
	if invErr != nil || isErr || content["ok"] != true {
		t.Fatalf("invoke content=%v isErr=%v err=%v", content, isErr, invErr)
	}
	if lastAuth != "Bearer SESSION" {
		t.Fatalf("Authorization=%q want Bearer SESSION (session identity over static)", lastAuth)
	}
}
