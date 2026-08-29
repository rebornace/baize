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

func TestLoadControlPlaneOperators(t *testing.T) {
	path := writeConfig(t, `control_plane:
  admin_token: env:BAIZE_ADMIN_TOKEN
  operators:
    - id: alice
      token: env:BAIZE_OP_ALICE
    - id: bob
      token: env:BAIZE_OP_BOB
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ControlPlane.Operators) != 2 {
		t.Fatalf("operators=%d", len(cfg.ControlPlane.Operators))
	}
	if cfg.ControlPlane.Operators[0].ID != "alice" || cfg.ControlPlane.Operators[0].Token != "env:BAIZE_OP_ALICE" {
		t.Fatalf("alice=%+v", cfg.ControlPlane.Operators[0])
	}
	if cfg.ControlPlane.Operators[1].ID != "bob" {
		t.Fatalf("bob=%+v", cfg.ControlPlane.Operators[1])
	}
}
