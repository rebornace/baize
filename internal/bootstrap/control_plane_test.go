package bootstrap

import (
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/config"
)

// TestNewAPIServerControlPlaneTokensEmpty: when no env vars are set and the
// YAML references env: vars, ResolveSecret returns "" so newAPIServer must
// NOT error and srv.OperatorToken/AdminToken must be empty (gate off).
func TestNewAPIServerControlPlaneTokensEmpty(t *testing.T) {
	cfg := minimalControlPlaneCfg(t)
	cfg.ControlPlane.OperatorToken = "env:BAIZE_OPERATOR_TOKEN"
	cfg.ControlPlane.AdminToken = "env:BAIZE_ADMIN_TOKEN"

	srv, closer, err := newAPIServer(cfg, "")
	if err != nil {
		t.Fatalf("newAPIServer: %v", err)
	}
	defer func() { _ = closer.Close() }()
	if srv.OperatorToken != "" {
		t.Fatalf("OperatorToken=%q want empty", srv.OperatorToken)
	}
	if srv.AdminToken != "" {
		t.Fatalf("AdminToken=%q want empty", srv.AdminToken)
	}
}

// TestNewAPIServerControlPlaneTokensResolved: setting the env var resolves the
// secret into srv.OperatorToken.
func TestNewAPIServerControlPlaneTokensResolved(t *testing.T) {
	t.Setenv("BAIZE_OPERATOR_TOKEN", "op1")
	t.Setenv("BAIZE_ADMIN_TOKEN", "adm1")

	cfg := minimalControlPlaneCfg(t)
	cfg.ControlPlane.OperatorToken = "env:BAIZE_OPERATOR_TOKEN"
	cfg.ControlPlane.AdminToken = "env:BAIZE_ADMIN_TOKEN"

	srv, closer, err := newAPIServer(cfg, "")
	if err != nil {
		t.Fatalf("newAPIServer: %v", err)
	}
	defer func() { _ = closer.Close() }()
	if srv.OperatorToken != "op1" {
		t.Fatalf("OperatorToken=%q want op1", srv.OperatorToken)
	}
	if srv.AdminToken != "adm1" {
		t.Fatalf("AdminToken=%q want adm1", srv.AdminToken)
	}
}

// TestNewAPIServerControlPlaneFileMissingFails: a file: ref pointing at a
// missing file must cause newAPIServer to return an error.
func TestNewAPIServerControlPlaneFileMissingFails(t *testing.T) {
	cfg := minimalControlPlaneCfg(t)
	cfg.ControlPlane.OperatorToken = "file:" + filepath.Join(t.TempDir(), "no-such-file")

	if _, _, err := newAPIServer(cfg, ""); err == nil {
		t.Fatal("expected error for missing operator token file, got nil")
	}
}

// TestNewAPIServerControlPlaneOperatorsResolved: named operator tokens resolve
// into srv.Operators for gate authentication.
func TestNewAPIServerControlPlaneOperatorsResolved(t *testing.T) {
	t.Setenv("BAIZE_ADMIN_TOKEN", "adm1")
	t.Setenv("BAIZE_OP_ALICE", "ta1")

	cfg := minimalControlPlaneCfg(t)
	cfg.ControlPlane.AdminToken = "env:BAIZE_ADMIN_TOKEN"
	cfg.ControlPlane.Operators = []config.OperatorEntry{
		{ID: "alice", Token: "env:BAIZE_OP_ALICE"},
		{ID: "bob", Token: "env:BAIZE_OP_BOB"},
	}

	srv, closer, err := newAPIServer(cfg, "")
	if err != nil {
		t.Fatalf("newAPIServer: %v", err)
	}
	defer func() { _ = closer.Close() }()
	if srv.AdminToken != "adm1" {
		t.Fatalf("AdminToken=%q want adm1", srv.AdminToken)
	}
	if len(srv.Operators) != 1 || srv.Operators[0].ID != "alice" || srv.Operators[0].Token != "ta1" {
		t.Fatalf("Operators=%+v", srv.Operators)
	}
}

// minimalControlPlaneCfg returns a config that builds an in-memory API server
// without touching real ports or a live upstream. The openapi spec is resolved
// relative to the repo root (test runs from internal/bootstrap).
func minimalControlPlaneCfg(t *testing.T) config.Config {
	t.Helper()
	cfg := config.Config{}
	cfg.Listen = ":0"
	cfg.Store.Driver = "memory"
	cfg.Agent.ID = "a"
	cfg.Connector.Type = "openapi"
	cfg.Connector.Spec = filepath.Join("..", "..", "examples", "mock-ticket", "openapi.yaml")
	cfg.Connector.BaseURL = "http://127.0.0.1:1"
	cfg.MockTicket.Listen = "off"
	return cfg
}
