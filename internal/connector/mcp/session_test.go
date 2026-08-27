package mcp_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
	"github.com/rebornace/baize/internal/store"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func buildMCPMock(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	name := "mcp-mock"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(root, "bin", name)
	cmd := exec.Command("go", "build", "-o", out, filepath.Join(root, "examples", "mcp-mock"))
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build mcp-mock: %v\n%s", err, output)
	}
	return out
}

func TestStdioDiscoverToolsEcho(t *testing.T) {
	mock := buildMCPMock(t)

	pool := &mcpbridge.SessionPool{}
	defer pool.CloseAll()

	ctx := context.Background()
	session, err := pool.OpenStdio(ctx, "test-stdio", mock, nil, nil)
	if err != nil {
		t.Fatalf("OpenStdio: %v", err)
	}

	tools, err := mcpbridge.DiscoverTools(ctx, session, "test-stdio")
	if err != nil {
		t.Fatalf("DiscoverTools: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected tools")
	}

	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
		if tool.Source != store.ToolSourceMCP {
			t.Fatalf("tool %q source=%q want %q", tool.Name, tool.Source, store.ToolSourceMCP)
		}
		if tool.ConnectorID != "test-stdio" {
			t.Fatalf("tool %q connector_id=%q", tool.Name, tool.ConnectorID)
		}
	}
	if !slices.Contains(names, "echo") {
		t.Fatalf("tools=%v, want echo", names)
	}
}
