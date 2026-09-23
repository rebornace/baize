package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/config"
)

func TestValidateRequiresAPIKey(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Validate(cfg); err == nil {
		t.Fatal("expected error when API key unset")
	}
	os.Setenv("BAIZE_API_KEY", "sk-test")
	defer os.Unsetenv("BAIZE_API_KEY")
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateRejectsMock(t *testing.T) {
	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	if err := config.Validate(cfg); err == nil {
		t.Fatal("expected error for mock provider on serve")
	}
}

func TestConfigYAMLProductionDefaults(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs", "config.yaml"))
	if err != nil {
		t.Fatal(err)
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
	if len(cfg.SkillBuiltinDirs()) != 1 || cfg.SkillBuiltinDirs()[0] != "./skills" {
		t.Fatalf("SkillBuiltinDirs=%v want [./skills]", cfg.SkillBuiltinDirs())
	}
}

func TestDockerMinimalYAML(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs", "docker-minimal.yaml"))
	if err != nil {
		t.Fatal(err)
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
