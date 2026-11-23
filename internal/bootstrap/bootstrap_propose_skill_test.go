package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/skill"
)

// TestNewAPIServerRegistersProposeSkill: bootstrap must register propose_skill
// on the shared registry with human approval when a skill catalog is wired.
func TestNewAPIServerRegistersProposeSkill(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
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

	srv, closer, err := newAPIServer(cfg, "")
	if err != nil {
		t.Fatalf("newAPIServer: %v", err)
	}
	defer func() { _ = closer.Close() }()

	info, ok := srv.Registry.Get(skill.ProposeName)
	if !ok {
		t.Fatalf("registry missing %q", skill.ProposeName)
	}
	if !info.RequireApproval {
		t.Fatalf("%q must require approval", skill.ProposeName)
	}
}
