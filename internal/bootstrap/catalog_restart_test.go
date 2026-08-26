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

// TestCatalogRestartSQLite: a connector's catalog (disabled rows + extra rows)
// persisted via the Store must survive a process restart. After reopening the
// store and running the bootstrap registration path (registerConnector for the
// YAML connector + loadStoredConnectors for the rest), the disabled row stays
// disabled and the extra row stays registered. The YAML connector itself must
// not have its disabled rows clobbered by re-Apply.
func TestCatalogRestartSQLite(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "baize.db")

	spec := writeBootstrapSpec(t)

	// Phase 1: open sqlite store, register the default YAML connector, then
	// disable one row and add an extra row.
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
	login := []string{}
	cfg.Connector.RequireLogin = login

	if err := registerConnector(st, reg, cfg, ids, connector.CallbackConfig{}); err != nil {
		t.Fatalf("registerConnector: %v", err)
	}
	if _, ok := reg.Get("probe"); !ok {
		t.Fatalf("probe should be registered after first registerConnector: %+v", reg.List())
	}

	// Disable probe in the catalog and add an extra row.
	probe, err := st.GetTool("probe")
	if err != nil {
		t.Fatalf("GetTool probe: %v", err)
	}
	probe.Enabled = false
	st.UpsertTool(probe)
	st.UpsertTool(store.Tool{
		ConnectorID: "yaml-c",
		Name:        "extraStatic",
		Source:      store.ToolSourceExtra,
		Enabled:     true,
		Method:      "GET",
		Path:        "/extra/static",
		Description: "extra static",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	})

	// Close the store (simulate shutdown).
	if c, ok := st.(interface{ Close() error }); ok {
		_ = c.Close()
	}

	// Phase 2: reopen the store and re-run the bootstrap registration path.
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

	// Disabled probe must not be registered.
	if _, ok := reg2.Get("probe"); ok {
		t.Fatalf("disabled probe must not be registered after restart: %+v", reg2.List())
	}
	// Disabled probe must still be in the store.
	if got, err := st2.GetTool("probe"); err != nil {
		t.Fatalf("probe missing from store after restart: %v", err)
	} else if got.Enabled {
		t.Fatalf("probe must remain disabled in store: %+v", got)
	}
	// Extra row must be registered.
	if _, ok := reg2.Get("extraStatic"); !ok {
		t.Fatalf("extraStatic must be registered after restart: %+v", reg2.List())
	}
	// Extra row must still be in the store.
	if got, err := st2.GetTool("extraStatic"); err != nil || got.Source != store.ToolSourceExtra {
		t.Fatalf("extraStatic missing in store: %+v err=%v", got, err)
	}

	// Close the reopened store so the temp dir can be cleaned up.
	if c, ok := st2.(interface{ Close() error }); ok {
		_ = c.Close()
	}
}
