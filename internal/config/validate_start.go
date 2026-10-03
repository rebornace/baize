package config

import (
	"fmt"
	"os"
	"strings"
)

// Validate checks that the config can run a real Runtime: an OpenAI-compatible
// LLM with its API key, base URL and model configured.
func Validate(cfg Config) error {
	prov := strings.ToLower(strings.TrimSpace(cfg.LLM.Provider))
	if prov == "" || prov == "mock" {
		return fmt.Errorf("a real LLM is required (llm.provider=openai_compatible); the mock provider is for tests")
	}
	if prov != "openai_compatible" {
		return fmt.Errorf("unsupported llm.provider %q (use openai_compatible)", cfg.LLM.Provider)
	}
	env := cfg.LLM.APIKeyEnv
	if env == "" {
		env = "BAIZE_API_KEY"
	}
	if strings.TrimSpace(os.Getenv(env)) == "" {
		return fmt.Errorf("%s is not set; export it (e.g. export BAIZE_API_KEY=sk-...) or set llm settings in the config", env)
	}
	if strings.TrimSpace(cfg.LLM.BaseURL) == "" {
		return fmt.Errorf("llm.base_url is required (set it in the config)")
	}
	if strings.TrimSpace(cfg.LLM.Model) == "" {
		return fmt.Errorf("llm.model is required (set it in the config)")
	}
	return nil
}
