package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/analysis"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/file"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func analysisPageToolArgs() map[string]any {
	return map[string]any{
		"title":  "Integration Demo",
		"format": "sections",
		"datasets": map[string]any{
			"d": map[string]any{
				"columns": []any{"c"},
				"rows":    []any{[]any{"1"}},
			},
		},
		"sections": []any{
			map[string]any{"type": "markdown", "content": "## Hi"},
		},
	}
}

// TestAnalysisPageE2E invokes create_analysis_page via a script LLM, asserts the
// run succeeds, then GET /v0/artifacts/{id} returns HTML containing page markers.
func TestAnalysisPageE2E(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "b.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	blobs, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	artStore, err := artifact.NewStore(blobs, st)
	if err != nil {
		t.Fatal(err)
	}

	st.UpsertAgent(store.Agent{ID: "analysis-agent", System: "helper"})
	reg := tool.NewRegistry()
	reg.RegisterSpec(analysis.ToolSpec(), analysis.Invoker(artStore))

	engine := &run.Engine{
		Store: st,
		LLM: &httpPluginScriptLLM{
			toolName: analysis.ToolName,
			args:     analysisPageToolArgs(),
		},
		Tools:    reg,
		Gate:     run.NewGate(),
		MaxSteps: 4,
	}
	srv := api.NewServer(st, reg, engine)
	srv.Artifacts = artStore
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	runBody := `{"agent_id":"analysis-agent","input":"create analysis page"}`
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

	var artifactID string
	for _, ev := range events {
		if ev.Type != run.EventToolResult {
			continue
		}
		name, _ := ev.Data["name"].(string)
		if name != analysis.ToolName {
			continue
		}
		content, _ := ev.Data["content"].(map[string]any)
		if content == nil {
			continue
		}
		id, _ := content["artifact_id"].(string)
		if id != "" {
			artifactID = id
			break
		}
	}
	if artifactID == "" {
		t.Fatalf("missing artifact_id in events: %+v", events)
	}

	artResp, err := http.Get(ts.URL + "/v0/artifacts/" + artifactID)
	if err != nil {
		t.Fatalf("GET artifact: %v", err)
	}
	defer artResp.Body.Close()
	htmlRaw, _ := io.ReadAll(artResp.Body)
	if artResp.StatusCode != http.StatusOK {
		t.Fatalf("GET artifact status=%d body=%s", artResp.StatusCode, htmlRaw)
	}
	html := string(htmlRaw)
	if !strings.Contains(html, "__BAIZE_PAGE__") && !strings.Contains(html, "BaizeReport") {
		t.Fatalf("artifact html missing page markers; body=%q", truncate(html, 500))
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
