package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/config"
)

func TestValidateStartRequiresAPIKey(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs", "minimal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := config.ValidateStart(cfg); err == nil {
		t.Fatal("expected error when API key unset")
	}
	os.Setenv("BAIZE_API_KEY", "sk-test")
	defer os.Unsetenv("BAIZE_API_KEY")
	if err := config.ValidateStart(cfg); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateStartRejectsMock(t *testing.T) {
	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	if err := config.ValidateStart(cfg); err == nil {
		t.Fatal("expected error for mock provider on start")
	}
}

func TestMinimalYAMLProductionDefaults(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs", "minimal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MockTicket.Listen != "off" {
		t.Fatalf("mock_ticket.listen=%q want off", cfg.MockTicket.Listen)
	}
	if cfg.LLM.Provider != "openai_compatible" {
		t.Fatalf("llm.provider=%q", cfg.LLM.Provider)
	}
	if cfg.Connector.ID != "" {
		t.Fatalf("connector.id=%q want empty", cfg.Connector.ID)
	}
	if cfg.Agent.ID != "default-agent" {
		t.Fatalf("agent.id=%q", cfg.Agent.ID)
	}
	if len(cfg.Agent.Skills) != 1 || cfg.Agent.Skills[0] != "data-analytics" {
		t.Fatalf("agent.skills=%v want [data-analytics]", cfg.Agent.Skills)
	}
	if cfg.Skills.BuiltinDir != "./skills" {
		t.Fatalf("skills.builtin_dir=%q want ./skills", cfg.Skills.BuiltinDir)
	}
}

func TestDemoYAMLTrialStack(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs", "demo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MockTicket.Listen != ":18080" {
		t.Fatalf("mock listen=%q", cfg.MockTicket.Listen)
	}
	if cfg.LLM.Provider != "mock" {
		t.Fatalf("llm.provider=%q", cfg.LLM.Provider)
	}
	if cfg.Connector.ID != "ticket-api" {
		t.Fatalf("connector.id=%q", cfg.Connector.ID)
	}
}

func TestDockerMinimalYAML(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs", "docker-minimal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MockTicket.Listen != "off" {
		t.Fatalf("mock_ticket.listen=%q want off", cfg.MockTicket.Listen)
	}
	if cfg.Connector.ID != "" {
		t.Fatalf("connector.id=%q want empty", cfg.Connector.ID)
	}
	if cfg.LLM.Provider != "openai_compatible" {
		t.Fatalf("llm.provider=%q", cfg.LLM.Provider)
	}
	if cfg.Skills.BuiltinDir != "/app/skills" {
		t.Fatalf("skills.builtin_dir=%q want /app/skills", cfg.Skills.BuiltinDir)
	}
	if len(cfg.Agent.Skills) != 1 || cfg.Agent.Skills[0] != "data-analytics" {
		t.Fatalf("agent.skills=%v want [data-analytics]", cfg.Agent.Skills)
	}
}

func TestDockerDemoYAML(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs", "docker-demo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MockTicket.Listen != "off" {
		t.Fatalf("mock_ticket.listen=%q", cfg.MockTicket.Listen)
	}
	if cfg.Connector.BaseURL != "http://mock-ticket:18080" {
		t.Fatalf("base_url=%q", cfg.Connector.BaseURL)
	}
	if cfg.LLM.Provider != "mock" {
		t.Fatalf("llm.provider=%q", cfg.LLM.Provider)
	}
}
