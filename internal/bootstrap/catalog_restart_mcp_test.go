package bootstrap

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func buildBootstrapMCPMock(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
	name := "mcp-mock"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(dir, "bin", name)
	cmd := exec.Command("go", "build", "-o", out, filepath.Join(dir, "examples", "mcp-mock"))
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build mcp-mock: %v\n%s", err, output)
	}
	return out
}

// TestCatalogRestartMCP: a persisted MCP connector must survive store reopen and
// loadStoredConnectors; echo remains registered and callable.
func TestCatalogRestartMCP(t *testing.T) {
	mock := buildBootstrapMCPMock(t)
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "baize.db")

	st, err := store.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	login := []string{}

	_, _, err = connector.Apply(connector.ApplyInput{
		Store:        st,
		Registry:     reg,
		Identities:   ids,
		ID:           "analytics",
		Type:         "mcp",
		MCP:          store.MCPConfig{Transport: "stdio", Command: mock},
		RequireLogin: &login,
	})
	if err != nil {
		t.Fatalf("Apply mcp connector: %v", err)
	}
	if _, ok := reg.Get("echo"); !ok {
		t.Fatalf("echo should be registered after Apply: %+v", reg.List())
	}

	if c, ok := st.(interface{ Close() error }); ok {
		_ = c.Close()
	}

	st2, err := store.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	reg2 := tool.NewRegistry()
	ids2 := identity.NewMemoryStore()
	cfg := config.Config{} // no YAML connector

	loadStoredConnectors(st2, reg2, cfg, ids2, connector.CallbackConfig{})

	if _, ok := reg2.Get("echo"); !ok {
		t.Fatalf("echo must be registered after loadStoredConnectors: %+v", reg2.List())
	}
	out, isErr, invErr := reg2.Invoke(context.Background(), "echo", map[string]any{"message": "restart"})
	if invErr != nil || isErr {
		t.Fatalf("invoke echo after restart: isErr=%v err=%v", isErr, invErr)
	}
	if out["message"] != "restart" {
		t.Fatalf("echo content=%+v", out)
	}

	if c, ok := st2.(interface{ Close() error }); ok {
		_ = c.Close()
	}
}
