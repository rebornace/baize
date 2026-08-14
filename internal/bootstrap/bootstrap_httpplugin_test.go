package bootstrap

import (
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
