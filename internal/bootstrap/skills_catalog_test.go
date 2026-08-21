package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewAPIServerInvalidSkillCatalogFails: invalid SKILL.md in builtin_dir must
// fail newAPIServer (same fail-fast path as production bootstrap).
func TestNewAPIServerInvalidSkillCatalogFails(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	badDir := filepath.Join(builtin, "bad-skill")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "SKILL.md"), []byte("no frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := minimalControlPlaneCfg(t)
	cfg.Skills.BuiltinDir = builtin
	cfg.Skills.UserDir = user

	if _, _, err := newAPIServer(cfg); err == nil {
		t.Fatal("expected error for invalid skill catalog, got nil")
	}
}
