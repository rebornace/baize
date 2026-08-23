package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func mcpBridgeRepoRoot(t *testing.T) string {
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
	root := mcpBridgeRepoRoot(t)
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

type mcpCatalogTool struct {
	Name        string `json:"name"`
	ConnectorID string `json:"connector_id"`
	Source      string `json:"source"`
}

func getMCPCatalogTools(t *testing.T, runtimeURL string) []mcpCatalogTool {
	t.Helper()
	resp, err := http.Get(runtimeURL + "/v0/tools")
	if err != nil {
		t.Fatalf("GET /v0/tools: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v0/tools status=%d body=%s", resp.StatusCode, raw)
	}
	var body struct {
		Tools []mcpCatalogTool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode tools: %v body=%s", err, raw)
	}
	return body.Tools
}

// TestMCPBridgeStdioPutToolsRun registers an MCP stdio connector via PUT,
// checks GET /v0/tools for echo with source=mcp, then POST /v0/runs with a
// script LLM that invokes echo and asserts tool.result in events.
func TestMCPBridgeStdioPutToolsRun(t *testing.T) {
	mock := buildMCPMockBinary(t)

	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "mcp-agent", System: "helper"})
	reg := tool.NewRegistry()
	engine := &run.Engine{
		Store:    st,
		LLM:      &httpPluginScriptLLM{toolName: "echo", args: map[string]any{"message": "hi"}},
		Tools:    reg,
		Gate:     run.NewGate(),
		MaxSteps: 4,
	}
	srv := api.NewServer(st, reg, engine)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	putConnector(t, ts.URL, "analytics", map[string]any{
		"type": "mcp",
		"mcp": map[string]any{
			"transport": "stdio",
			"command":   mock,
		},
	}, http.StatusOK)

	tools := getMCPCatalogTools(t, ts.URL)
	var foundEcho bool
	for _, info := range tools {
		if info.Name == "echo" && info.ConnectorID == "analytics" && info.Source == store.ToolSourceMCP {
			foundEcho = true
			break
		}
	}
	if !foundEcho {
		t.Fatalf("expected echo with connector_id=analytics source=mcp; tools=%+v", tools)
	}

	runBody := `{"agent_id":"mcp-agent","input":"please echo"}`
	resp, err := http.Post(ts.URL+"/v0/runs", "application/json", strings.NewReader(runBody))
	if err != nil {
		t.Fatalf("POST /v0/runs: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v0/runs status=%d body=%s", resp.StatusCode, raw)
	}
	var created struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("decode run: %v body=%s", err, raw)
	}
	if created.RunID == "" {
		t.Fatalf("missing run_id: %s", raw)
	}

	pollRun(t, ts.URL, created.RunID, store.StatusSucceeded)

	evResp, err := http.Get(ts.URL + "/v0/runs/" + created.RunID + "/events")
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer evResp.Body.Close()
	evRaw, _ := io.ReadAll(evResp.Body)
	if evResp.StatusCode != http.StatusOK {
		t.Fatalf("GET events status=%d body=%s", evResp.StatusCode, evRaw)
	}
	var events []store.Event
	if err := json.Unmarshal(evRaw, &events); err != nil {
		t.Fatalf("decode events: %v body=%s", err, evRaw)
	}
	hasToolResult := false
	for _, ev := range events {
		if ev.Type == run.EventToolResult {
			hasToolResult = true
			break
		}
	}
	if !hasToolResult {
		t.Fatalf("events missing tool.result: %+v", events)
	}
}
