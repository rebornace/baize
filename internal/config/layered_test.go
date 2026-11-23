package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLayeredMergesOverlay(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	overlay := filepath.Join(dir, "overlay.yaml")
	if err := os.WriteFile(base, []byte(`
llm:
  provider: mock
agent:
  id: ticket-agent
  skills:
    - data-analytics
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlay, []byte(`
llm:
  provider: openai_compatible
  base_url: https://api.example.com
  model: test-model
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, applied, err := LoadLayered(base, overlay)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 2 {
		t.Fatalf("applied=%v", applied)
	}
	if cfg.LLM.Provider != "openai_compatible" {
		t.Fatalf("llm.provider=%q", cfg.LLM.Provider)
	}
	if cfg.LLM.Model != "test-model" {
		t.Fatalf("llm.model=%q", cfg.LLM.Model)
	}
	if cfg.Agent.ID != "ticket-agent" {
		t.Fatalf("agent.id=%q want ticket-agent from base", cfg.Agent.ID)
	}
	if len(cfg.Agent.Skills) != 1 || cfg.Agent.Skills[0] != "data-analytics" {
		t.Fatalf("agent.skills=%v", cfg.Agent.Skills)
	}
}

func TestLoadLayeredSkipsMissingOverlay(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	if err := os.WriteFile(base, []byte(`
llm:
  provider: mock
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, applied, err := LoadLayered(base, filepath.Join(dir, "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 {
		t.Fatalf("applied=%v", applied)
	}
	if cfg.LLM.Provider != "mock" {
		t.Fatalf("provider=%q", cfg.LLM.Provider)
	}
}
