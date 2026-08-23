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
	if len(cfg.SkillBuiltinDirs()) != 1 || cfg.SkillBuiltinDirs()[0] != "./skills" {
		t.Fatalf("SkillBuiltinDirs=%v want [./skills]", cfg.SkillBuiltinDirs())
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
	if cfg.Skills.BuiltinDir != "" {
		t.Fatalf("skills.builtin_dir=%q want empty when builtin_dirs set", cfg.Skills.BuiltinDir)
	}
	wantDirs := []string{"./skills", "./examples/skills"}
	gotDirs := cfg.SkillBuiltinDirs()
	if len(gotDirs) != len(wantDirs) {
		t.Fatalf("SkillBuiltinDirs=%v want %v", gotDirs, wantDirs)
	}
	for i, w := range wantDirs {
		if gotDirs[i] != w {
			t.Fatalf("SkillBuiltinDirs[%d]=%q want %q", i, gotDirs[i], w)
		}
	}
	wantSkills := []string{"data-analytics", "ticket-triage"}
	if len(cfg.Agent.Skills) != len(wantSkills) {
		t.Fatalf("agent.skills=%v want %v", cfg.Agent.Skills, wantSkills)
	}
	for i, w := range wantSkills {
		if cfg.Agent.Skills[i] != w {
			t.Fatalf("agent.skills[%d]=%q want %q", i, cfg.Agent.Skills[i], w)
		}
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
	if cfg.Skills.BuiltinDir != "" {
		t.Fatalf("skills.builtin_dir=%q want empty when builtin_dirs set", cfg.Skills.BuiltinDir)
	}
	wantDirs := []string{"/app/skills", "/app/examples/skills"}
	gotDirs := cfg.SkillBuiltinDirs()
	if len(gotDirs) != len(wantDirs) {
		t.Fatalf("SkillBuiltinDirs=%v want %v", gotDirs, wantDirs)
	}
	for i, w := range wantDirs {
		if gotDirs[i] != w {
			t.Fatalf("SkillBuiltinDirs[%d]=%q want %q", i, gotDirs[i], w)
		}
	}
	wantSkills := []string{"data-analytics", "ticket-triage"}
	if len(cfg.Agent.Skills) != len(wantSkills) {
		t.Fatalf("agent.skills=%v want %v", cfg.Agent.Skills, wantSkills)
	}
	for i, w := range wantSkills {
		if cfg.Agent.Skills[i] != w {
			t.Fatalf("agent.skills[%d]=%q want %q", i, cfg.Agent.Skills[i], w)
		}
	}
}
