package config_test

import (
	"testing"

	"github.com/rebornace/baize/internal/config"
)

func TestLoadControlPlaneTokens(t *testing.T) {
	path := writeConfig(t, "control_plane:\n  operator_token: env:BAIZE_OPERATOR_TOKEN\n  admin_token: env:BAIZE_ADMIN_TOKEN\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ControlPlane.OperatorToken != "env:BAIZE_OPERATOR_TOKEN" {
		t.Fatalf("op=%q", cfg.ControlPlane.OperatorToken)
	}
	if cfg.ControlPlane.AdminToken != "env:BAIZE_ADMIN_TOKEN" {
		t.Fatalf("adm=%q", cfg.ControlPlane.AdminToken)
	}
}
