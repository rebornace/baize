package connector_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rebornace/baize/internal/connector"
	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func mcpRepoRoot(t *testing.T) string {
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

func buildMCPMockBinary(t *testing.T) string {
	t.Helper()
	root := mcpRepoRoot(t)
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

func TestApplyMCPStdioDiscoversAndInvokesEcho(t *testing.T) {
	mock := buildMCPMockBinary(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	login := []string{}

	_, infos, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "mcp1", Type: "mcp",
		MCP: store.MCPConfig{
			Transport: "stdio",
			Command:   mock,
		},
		RequireLogin: &login,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rows := st.ListToolsByConnector("mcp1")
	if len(rows) == 0 {
		t.Fatal("expected catalog rows")
	}
	foundMCP := false
	for _, row := range rows {
		if row.Source == store.ToolSourceMCP {
			foundMCP = true
		}
	}
	if !foundMCP {
		t.Fatalf("expected source=mcp row, got %+v", rows)
	}

	echoRegistered := false
	for _, info := range infos {
		if info.Name == "echo" {
			echoRegistered = true
		}
	}
	if !echoRegistered {
		t.Fatalf("echo not in registry infos: %+v", infos)
	}

	out, isErr, invErr := reg.Invoke(context.Background(), "echo", map[string]any{"message": "hi"})
	if invErr != nil || isErr {
		t.Fatalf("invoke echo: isErr=%v err=%v", isErr, invErr)
	}
	if out["message"] != "hi" {
		t.Fatalf("echo content=%+v", out)
	}
}

func TestApplyMCPBadCommandPreservesRegistry(t *testing.T) {
	mock := buildMCPMockBinary(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	login := []string{}

	base := connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "mcp1", Type: "mcp",
		MCP: store.MCPConfig{
			Transport: "stdio",
			Command:   mock,
		},
		RequireLogin: &login,
	}
	if _, _, err := connector.Apply(base); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	before := reg.List()

	bad := base
	bad.MCP = store.MCPConfig{
		Transport: "stdio",
		Command:   filepath.Join(t.TempDir(), "nonexistent-mcp-binary"),
	}
	_, _, err := connector.Apply(bad)
	if err == nil {
		t.Fatal("expected error from bad mcp command")
	}
	if !errors.Is(err, mcpbridge.ErrInvalidMCP) {
		t.Fatalf("expected ErrInvalidMCP, got %v", err)
	}

	after := reg.List()
	if len(after) != len(before) {
		t.Fatalf("registry changed: before=%+v after=%+v", before, after)
	}
}

func TestRegisterOneFromConnectorRejectsMCPExtra(t *testing.T) {
	mock := buildMCPMockBinary(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	login := []string{}
	if _, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "mcp1", Type: "mcp",
		MCP: store.MCPConfig{
			Transport: "stdio",
			Command:   mock,
		},
		RequireLogin: &login,
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	c, err := st.GetConnector("mcp1")
	if err != nil {
		t.Fatal(err)
	}
	extra := store.Tool{
		ConnectorID: "mcp1",
		Name:        "phantom",
		Source:      store.ToolSourceExtra,
		Enabled:     true,
		Method:      "GET",
		Path:        "/phantom",
		InputSchema: map[string]any{"type": "object"},
	}
	if err := connector.RegisterOneFromConnector(st, reg, ids, c, extra); err == nil {
		t.Fatal("expected error registering extra on mcp connector")
	}
}
