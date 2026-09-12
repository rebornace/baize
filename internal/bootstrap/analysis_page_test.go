package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/analysis"
)

// Regression: eventbus.Notify wraps the store; artifact registration must still
// register create_analysis_page when using the sqlite driver.
func TestNewAPIServerRegistersAnalysisPageWithSQLite(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	userDir := filepath.Join(root, "user")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := minimalControlPlaneCfg(t)
	cfg.Store.Driver = "sqlite"
	cfg.Store.SQLitePath = filepath.Join(root, "baize.db")
	cfg.Skills.BuiltinDir = skillsDir
	cfg.Skills.UserDir = userDir

	srv, closer, err := newAPIServer(cfg, "")
	if err != nil {
		t.Fatalf("newAPIServer: %v", err)
	}
	defer func() { _ = closer.Close() }()

	if srv.Artifacts == nil {
		t.Fatal("Artifacts store is nil; create_analysis_page was not wired")
	}
	if srv.Registry == nil {
		t.Fatal("Registry is nil")
	}
	found := false
	for _, spec := range srv.Registry.Specs() {
		if spec.Name == analysis.ToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("tool %q not registered; specs=%v", analysis.ToolName, srv.Registry.Specs())
	}
}
