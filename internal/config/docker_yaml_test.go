package config_test

import (
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/config"
)

func TestDockerYAMLSidecarCompose(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs", "docker.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MockTicket.Listen != "off" {
		t.Fatalf("mock_ticket.listen=%q want off", cfg.MockTicket.Listen)
	}
	if cfg.Connector.BaseURL != "http://mock-ticket:18080" {
		t.Fatalf("base_url=%q", cfg.Connector.BaseURL)
	}
	if cfg.Store.SQLitePath != "/app/data/baize.db" {
		t.Fatalf("sqlite=%q", cfg.Store.SQLitePath)
	}
	if cfg.Connector.Spec != "examples/mock-ticket/openapi.yaml" {
		t.Fatalf("spec=%q", cfg.Connector.Spec)
	}
	if cfg.LLM.Provider != "mock" || cfg.Agent.ID != "ticket-agent" {
		t.Fatalf("llm/agent %+v %+v", cfg.LLM, cfg.Agent)
	}
	if len(cfg.Connector.Auth.Static.Headers["Authorization"]) > 0 {
		t.Fatalf("open-box yaml must not require connector token header: %+v", cfg.Connector.Auth.Static.Headers)
	}
}

func TestDefaultYAMLUnchangedForLocalStart(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs", "default.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MockTicket.Listen != ":18080" {
		t.Fatalf("default mock listen=%q", cfg.MockTicket.Listen)
	}
	if cfg.Connector.BaseURL != "http://127.0.0.1:18080" {
		t.Fatalf("default base_url=%q", cfg.Connector.BaseURL)
	}
	if cfg.Connector.Type != "openapi" {
		t.Fatalf("type=%q", cfg.Connector.Type)
	}
	if len(cfg.Connector.Auth.Static.Headers["Authorization"]) > 0 {
		t.Fatalf("open-box yaml must not require connector token header: %+v", cfg.Connector.Auth.Static.Headers)
	}
}
