package bootstrap

import (
	"testing"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/llm"
)

// TestNewLLMMockInjectsSupportsVision verifies that cfg.LLM.SupportsVision is
// propagated to the Mock provider, so YAML `supports_vision: true` is honored
// even when no real LLM backend is configured.
func TestNewLLMMockInjectsSupportsVision(t *testing.T) {
	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.LLM.SupportsVision = true

	p, err := newLLM(cfg)
	if err != nil {
		t.Fatalf("newLLM: %v", err)
	}
	m, ok := p.(*llm.Mock)
	if !ok {
		t.Fatalf("expected *llm.Mock, got %T", p)
	}
	if !m.SupportsVision() {
		t.Fatalf("mock SupportsVision() = false, want true (cfg.LLM.SupportsVision=true)")
	}

	// Default (false) path stays false.
	cfg2 := config.Config{}
	cfg2.LLM.Provider = "mock"
	p2, err := newLLM(cfg2)
	if err != nil {
		t.Fatalf("newLLM default: %v", err)
	}
	if p2.SupportsVision() {
		t.Fatalf("default mock SupportsVision() = true, want false")
	}
}

// TestNewLLMOpenAIInjectsSupportsVision verifies the openai_compatible path
// still propagates the flag (regression guard alongside the Mock fix).
func TestNewLLMOpenAIInjectsSupportsVision(t *testing.T) {
	cfg := config.Config{}
	cfg.LLM.Provider = "openai_compatible"
	cfg.LLM.BaseURL = "http://127.0.0.1:0"
	cfg.LLM.Model = "m"
	cfg.LLM.SupportsVision = true

	p, err := newLLM(cfg)
	if err != nil {
		t.Fatalf("newLLM: %v", err)
	}
	o, ok := p.(*llm.OpenAI)
	if !ok {
		t.Fatalf("expected *llm.OpenAI, got %T", p)
	}
	if !o.SupportsVision() {
		t.Fatalf("openai SupportsVision() = false, want true")
	}
}
