package config_test

import (
	"testing"

	"github.com/rebornace/baize/internal/config"
)

func TestLoadSkillsDirDefaults(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Skills.BuiltinDir != "" {
		t.Fatalf("BuiltinDir=%q want empty default", cfg.Skills.BuiltinDir)
	}
	if cfg.Skills.UserDir != "./data/skills" {
		t.Fatalf("UserDir=%q want ./data/skills", cfg.Skills.UserDir)
	}
}

func TestLoadAgentSkillsExplicit(t *testing.T) {
	path := writeConfig(t, "store:\n  driver: memory\nagent:\n  id: ticket-agent\n  skills:\n    - ticket-triage\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Agent.Skills) != 1 || cfg.Agent.Skills[0] != "ticket-triage" {
		t.Fatalf("Agent.Skills=%v", cfg.Agent.Skills)
	}
}
