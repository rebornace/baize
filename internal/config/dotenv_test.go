package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/config"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("BAIZE_API_KEY=sk-test\n# comment\nOTHER='quoted'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BAIZE_API_KEY", "")
	_ = os.Unsetenv("BAIZE_API_KEY")
	_ = os.Unsetenv("OTHER")

	if err := config.LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("BAIZE_API_KEY"); got != "sk-test" {
		t.Fatalf("BAIZE_API_KEY=%q", got)
	}
	if got := os.Getenv("OTHER"); got != "quoted" {
		t.Fatalf("OTHER=%q", got)
	}

	t.Setenv("BAIZE_API_KEY", "keep-me")
	if err := config.LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("BAIZE_API_KEY"); got != "keep-me" {
		t.Fatalf("should not overwrite existing env, got %q", got)
	}
}

func TestLoadDotEnvMissing(t *testing.T) {
	if err := config.LoadDotEnv(filepath.Join(t.TempDir(), "nope.env")); err != nil {
		t.Fatal(err)
	}
}
