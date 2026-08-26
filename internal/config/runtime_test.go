package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeDefaultsApplied(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(p, []byte("llm:\n  provider: mock\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runtime.CallbackTokenTTLSec != DefaultCallbackTokenTTLSec {
		t.Fatalf("ttl=%d want %d", cfg.Runtime.CallbackTokenTTLSec, DefaultCallbackTokenTTLSec)
	}
	if cfg.Runtime.PublicBaseURL != "" {
		t.Fatalf("public_base_url=%q want empty by default", cfg.Runtime.PublicBaseURL)
	}
	if cfg.Runtime.CallbackHMACSecret != "" {
		t.Fatalf("callback_hmac_secret=%q want empty by default", cfg.Runtime.CallbackHMACSecret)
	}
}

func TestRuntimeOverridesPreserved(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(p, []byte(`
llm:
  provider: mock
runtime:
  public_base_url: https://baize.internal:8080
  callback_hmac_secret: configured-secret
  callback_token_ttl_sec: 600
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Runtime.PublicBaseURL != "https://baize.internal:8080" {
		t.Fatalf("public_base_url=%q", cfg.Runtime.PublicBaseURL)
	}
	if cfg.Runtime.CallbackHMACSecret != "configured-secret" {
		t.Fatalf("secret=%q", cfg.Runtime.CallbackHMACSecret)
	}
	if cfg.Runtime.CallbackTokenTTLSec != 600 {
		t.Fatalf("ttl=%d want 600", cfg.Runtime.CallbackTokenTTLSec)
	}
}
