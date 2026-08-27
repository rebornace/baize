package bootstrap

import (
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// TestCatalogRestartOmittedRequireLoginPreservesPerToolFlag: YAML 省略
// require_login 时 registerConnector 必须传 RequireLogin=nil，让 MergeCatalog
// 保留行上已持久化的 require_login。模拟设置页 PATCH require_login=true 后
// 关进程重启：probe 行的 require_login 必须仍为 true。
func TestCatalogRestartOmittedRequireLoginPreservesPerToolFlag(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "baize.db")

	spec := writeBootstrapSpec(t)

	// Phase 1: register with YAML omitting require_login (nil).
	st, err := store.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()

	cfg := config.Config{}
	cfg.Connector.ID = "yaml-c"
	cfg.Connector.Type = "openapi"
	cfg.Connector.Spec = spec
	cfg.Connector.BaseURL = "http://example.invalid"
	cfg.Connector.Auth.Mode = "static"
	// require_login 省略 → cfg.Connector.RequireLogin == nil

	if err := registerConnector(st, reg, cfg, ids, connector.CallbackConfig{}); err != nil {
		t.Fatalf("registerConnector: %v", err)
	}
	if _, ok := reg.Get("probe"); !ok {
		t.Fatalf("probe should be registered: %+v", reg.List())
	}

	// Simulate settings page PATCH require_login=true on probe.
	probe, err := st.GetTool("probe")
	if err != nil {
		t.Fatalf("GetTool probe: %v", err)
	}
	probe.RequireLogin = true
	st.UpsertTool(probe)

	// Close (simulate shutdown).
	if c, ok := st.(interface{ Close() error }); ok {
		_ = c.Close()
	}

	// Phase 2: reopen and re-run bootstrap registration path.
	st2, err := store.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	reg2 := tool.NewRegistry()
	ids2 := identity.NewMemoryStore()

	if err := registerConnector(st2, reg2, cfg, ids2, connector.CallbackConfig{}); err != nil {
		t.Fatalf("registerConnector after restart: %v", err)
	}
	loadStoredConnectors(st2, reg2, cfg, ids2, connector.CallbackConfig{})

	// Per-tool require_login must survive the restart.
	got, err := st2.GetTool("probe")
	if err != nil {
		t.Fatalf("probe missing after restart: %v", err)
	}
	if !got.RequireLogin {
		t.Fatalf("probe.RequireLogin must remain true after restart (YAML omitted require_login): %+v", got)
	}
	// Registry must also reflect the flag.
	if !reg2.RequiresLogin("probe") {
		t.Fatalf("Registry.RequiresLogin(probe) must be true after restart: %+v", reg2.List())
	}

	if c, ok := st2.(interface{ Close() error }); ok {
		_ = c.Close()
	}
}
