package config

import (
	"fmt"
	"os"
	"strings"
)

// ValidateStart checks production defaults for baize start (minimal config, real LLM).
func ValidateStart(cfg Config) error {
	prov := strings.ToLower(strings.TrimSpace(cfg.LLM.Provider))
	if prov == "" || prov == "mock" {
		return fmt.Errorf("baize start requires a real LLM (openai_compatible in configs/minimal.yaml); use baize demo for the mock trial stack")
	}
	if prov != "openai_compatible" {
		return fmt.Errorf("baize start: unsupported llm.provider %q (use openai_compatible or baize demo)", cfg.LLM.Provider)
	}
	env := cfg.LLM.APIKeyEnv
	if env == "" {
		env = "BAIZE_API_KEY"
	}
	if strings.TrimSpace(os.Getenv(env)) == "" {
		return fmt.Errorf("%s is not set\n\nExport your API key before baize start, for example:\n  export BAIZE_API_KEY=sk-...\n\nOr copy configs/minimal.yaml to configs/minimal.local.yaml and set llm settings there.", env)
	}
	if strings.TrimSpace(cfg.LLM.BaseURL) == "" {
		return fmt.Errorf("llm.base_url is required (set in configs/minimal.yaml or configs/minimal.local.yaml)")
	}
	if strings.TrimSpace(cfg.LLM.Model) == "" {
		return fmt.Errorf("llm.model is required (set in configs/minimal.yaml or configs/minimal.local.yaml)")
	}
	return nil
}
