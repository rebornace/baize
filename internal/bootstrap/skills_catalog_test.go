package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/run"
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

// TestNewAPIServerWiresEngineSkills: production bootstrap must assign the same
// catalog pointer to Engine.Skills so Run-time filtering / activate_skill work.
func TestNewAPIServerWiresEngineSkills(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	if err := os.MkdirAll(builtin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(user, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := minimalControlPlaneCfg(t)
	cfg.Skills.BuiltinDir = builtin
	cfg.Skills.UserDir = user

	srv, closer, err := newAPIServer(cfg)
	if err != nil {
		t.Fatalf("newAPIServer: %v", err)
	}
	defer func() { _ = closer.Close() }()

	if srv.SkillCatalog == nil {
		t.Fatal("SkillCatalog is nil")
	}
	eng, ok := srv.Runner.(*run.Engine)
	if !ok {
		t.Fatalf("Runner type %T, want *run.Engine", srv.Runner)
	}
	if eng.Skills == nil {
		t.Fatal("engine.Skills is nil")
	}
	if eng.Skills != srv.SkillCatalog {
		t.Fatal("engine.Skills must be the same catalog pointer as srv.SkillCatalog")
	}
}
