package toolretrieval

import (
	"path/filepath"
	"testing"
)

func TestResolveModelsDirPrecedence(t *testing.T) {
	t.Setenv("OLLAMA_MODELS", "")
	dir, src := ResolveModelsDir("")
	if src != "default" || dir == "" {
		t.Fatalf("default: dir=%q src=%q", dir, src)
	}

	custom := filepath.Join(t.TempDir(), "models")
	dir, src = ResolveModelsDir(custom)
	if src != "baize" || dir != filepath.Clean(custom) {
		t.Fatalf("baize: dir=%q src=%q", dir, src)
	}

	envDir := filepath.Join(t.TempDir(), "env-models")
	t.Setenv("OLLAMA_MODELS", envDir)
	dir, src = ResolveModelsDir("")
	if src != "env" || dir != filepath.Clean(envDir) {
		t.Fatalf("env: dir=%q src=%q", dir, src)
	}
	// Baize override still wins over env.
	dir, src = ResolveModelsDir(custom)
	if src != "baize" || dir != filepath.Clean(custom) {
		t.Fatalf("baize>env: dir=%q src=%q", dir, src)
	}
}

func TestDiscoverPathsIncludesCache(t *testing.T) {
	p := DiscoverPaths("")
	if p.ModelsDir == "" {
		t.Fatal("models_dir empty")
	}
	if p.InstallerCacheDir == "" {
		t.Fatal("installer_cache_dir empty")
	}
	if p.ModelsDirSource != "default" && p.ModelsDirSource != "env" {
		t.Fatalf("unexpected source %q", p.ModelsDirSource)
	}
}
