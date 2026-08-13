package config_test

import (
	"testing"

	"github.com/rebornace/baize/internal/config"
)

func TestLoadAuthModeDefaultStatic(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\nconnector:\n  auth:\n    static:\n      headers:\n        Authorization: Bearer ${BAIZE_CONNECTOR_TOKEN}\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Connector.Auth.Mode != "" && cfg.Connector.Auth.Mode != "static" {
		t.Fatalf("mode=%q", cfg.Connector.Auth.Mode)
	}
	if cfg.Connector.Auth.Static.Headers["Authorization"] != "Bearer ${BAIZE_CONNECTOR_TOKEN}" {
		t.Fatalf("headers=%v", cfg.Connector.Auth.Static.Headers)
	}
}

func TestLoadAuthVaultRef(t *testing.T) {
	path := writeConfig(t, "connector:\n  auth:\n    mode: vault_ref\n    vault_ref:\n      headers:\n        Authorization: env:BAIZE_CONNECTOR_TOKEN\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Connector.Auth.Mode != "vault_ref" {
		t.Fatalf("mode=%q", cfg.Connector.Auth.Mode)
	}
	if cfg.Connector.Auth.VaultRef.Headers["Authorization"] != "env:BAIZE_CONNECTOR_TOKEN" {
		t.Fatalf("%v", cfg.Connector.Auth.VaultRef.Headers)
	}
}
