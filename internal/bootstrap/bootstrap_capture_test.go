package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// writeBootstrapSpec writes a minimal OpenAPI spec with one GET op for bootstrap tests.
func writeBootstrapSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "spec.yaml")
	content := "openapi: 3.0.3\n" +
		"info:\n  title: bs\n  version: 0.1.0\n" +
		"paths:\n  /probe:\n    get:\n      operationId: probe\n" +
		"      responses:\n        \"200\":\n          description: ok\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestRegisterConnectorStaticMissingEnvFails: ResolveDefaults on a static
// header referencing an unset env var must return an error so the runtime
// fails to start rather than silently registering with no auth.
func TestRegisterConnectorStaticMissingEnvFails(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	cfg := config.Config{}
	cfg.Connector.ID = "bs"
	cfg.Connector.Type = "openapi"
	cfg.Connector.Spec = writeBootstrapSpec(t)
	cfg.Connector.BaseURL = "http://example.invalid"
	cfg.Connector.Auth.Mode = "static"
	cfg.Connector.Auth.Static.Headers = map[string]string{
		"Authorization": "Bearer ${BOOTSTRAP_MISSING_ENV_XYZ}",
	}

	err := registerConnector(st, reg, cfg, identity.NewMemoryStore(), connector.CallbackConfig{})
	if err == nil {
		t.Fatal("expected error for unresolved static env, got nil")
	}
	if !strings.Contains(err.Error(), "resolve connector auth") {
		t.Fatalf("err=%v want prefix resolve connector auth", err)
	}
	// Connector must not have been upserted on failure.
	if _, err := st.GetConnector("bs"); err == nil {
		t.Fatal("connector persisted despite auth resolution failure")
	}
}

// TestRegisterConnectorVaultRefPersistsAuthShape: a successful vault_ref
// registration must persist the Auth config shape (mode + references, not
// resolved secrets) onto the store Connector.
func TestRegisterConnectorVaultRefPersistsAuthShape(t *testing.T) {
	t.Setenv("BOOTSTRAP_VAULT_TOK", "secret-value")
	st := store.NewMemory()
	reg := tool.NewRegistry()
	cfg := config.Config{}
	cfg.Connector.ID = "bs-vault"
	cfg.Connector.Type = "openapi"
	cfg.Connector.Spec = writeBootstrapSpec(t)
	cfg.Connector.BaseURL = "http://example.invalid"
	cfg.Connector.Auth.Mode = "vault_ref"
	cfg.Connector.Auth.VaultRef.Headers = map[string]string{
		"Authorization": "env:BOOTSTRAP_VAULT_TOK",
	}

	if err := registerConnector(st, reg, cfg, identity.NewMemoryStore(), connector.CallbackConfig{}); err != nil {
		t.Fatalf("registerConnector: %v", err)
	}
	c, err := st.GetConnector("bs-vault")
	if err != nil {
		t.Fatal(err)
	}
	if c.Auth.Mode != "vault_ref" {
		t.Fatalf("Auth.Mode=%q want vault_ref", c.Auth.Mode)
	}
	if c.Auth.VaultRef.Headers["Authorization"] != "env:BOOTSTRAP_VAULT_TOK" {
		t.Fatalf("Auth.VaultRef.Headers=%+v want reference env:BOOTSTRAP_VAULT_TOK", c.Auth.VaultRef.Headers)
	}
	// Resolved secret must not be persisted on the connector.
	for _, h := range c.Auth.VaultRef.Headers {
		if strings.Contains(h, "secret-value") {
			t.Fatalf("resolved secret leaked into store connector: %+v", c.Auth.VaultRef.Headers)
		}
	}
}
