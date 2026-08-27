package analysis_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/analysis"
	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
)

func testFileStore(t *testing.T) artifact.Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "b.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	fs, err := artifact.NewFileStore(filepath.Join(dir, "artifacts"), st)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestCreateAnalysisPageInvoke(t *testing.T) {
	art := testFileStore(t)
	inv := analysis.Invoker(art)
	out, isErr, err := inv(identity.WithRunID(context.Background(), "run_x"), map[string]any{
		"title": "T", "format": "sections",
		"datasets": map[string]any{
			"d": map[string]any{"columns": []any{"c"}, "rows": []any{[]any{"1"}}},
		},
		"sections": []any{map[string]any{"type": "markdown", "content": "hi"}},
	})
	if err != nil || isErr {
		t.Fatalf("invoke: err=%v isErr=%v", err, isErr)
	}
	if out["kind"] != "analysis_page" || out["artifact_url"] == "" {
		t.Fatalf("out=%v", out)
	}
	if out["artifact_id"] == "" {
		t.Fatalf("missing artifact_id: out=%v", out)
	}
	if out["format"] != "sections" {
		t.Fatalf("format=%v", out["format"])
	}
	if out["section_count"] != 1 {
		t.Fatalf("section_count=%v", out["section_count"])
	}
}

func TestCreateAnalysisPageValidationError(t *testing.T) {
	art := testFileStore(t)
	inv := analysis.Invoker(art)
	out, isErr, err := inv(context.Background(), map[string]any{
		"format": "sections",
		"sections": []any{
			map[string]any{"type": "echarts"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !isErr {
		t.Fatalf("expected isErr, out=%v", out)
	}
	if out["error"] == nil {
		t.Fatalf("expected error in content: out=%v", out)
	}
}

func TestToolSpec(t *testing.T) {
	spec := analysis.ToolSpec()
	if spec.Name != analysis.ToolName {
		t.Fatalf("name=%q", spec.Name)
	}
	if spec.InputSchema == nil {
		t.Fatal("missing input schema")
	}
}
