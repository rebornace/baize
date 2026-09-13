package report_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/report"
)

func TestValidateEchartsMissingOptionAndBinding(t *testing.T) {
	req := &report.PageRequest{
		Format: "sections",
		Datasets: map[string]report.Dataset{
			"tickets": {Columns: []string{"id"}, Rows: [][]any{{"T1"}}},
		},
		Sections: []report.Section{
			{Type: "echarts", ID: "chart_status", Title: "按状态"},
		},
	}
	if err := report.Validate(req); err == nil {
		t.Fatal("expected error for echarts without option or binding")
	}
}

func TestValidateBindingDatasetNotFound(t *testing.T) {
	req := &report.PageRequest{
		Format: "sections",
		Datasets: map[string]report.Dataset{
			"tickets": {Columns: []string{"status"}, Rows: [][]any{{"open"}}},
		},
		Sections: []report.Section{
			{
				Type: "table",
				Binding: &report.Binding{
					Dataset: "missing",
					Columns: []string{"status"},
				},
			},
		},
	}
	if err := report.Validate(req); err == nil {
		t.Fatal("expected error for unknown binding.dataset")
	}
}

func TestValidateDatasetRowWidthMismatch(t *testing.T) {
	req := &report.PageRequest{
		Format: "sections",
		Datasets: map[string]report.Dataset{
			"tickets": {
				Columns: []string{"id", "status", "priority"},
				Rows:    [][]any{{"T1", "open"}},
			},
		},
		Sections: []report.Section{
			{Type: "markdown", Content: "## ok"},
		},
	}
	if err := report.Validate(req); err == nil {
		t.Fatal("expected error for dataset row width mismatch")
	}
}

func TestValidateHTMLTooLarge(t *testing.T) {
	req := &report.PageRequest{
		Format: "html",
		HTML:   strings.Repeat("x", 512*1024+1),
	}
	if err := report.Validate(req); err == nil {
		t.Fatal("expected error for html exceeding 512 KiB")
	}
}

func TestValidateHTMLExternalScriptSrc(t *testing.T) {
	req := &report.PageRequest{
		Format: "html",
		HTML:   `<!DOCTYPE html><html><body><script src="https://evil.example/x.js"></script></body></html>`,
	}
	if err := report.Validate(req); err == nil {
		t.Fatal("expected error for external script src in html")
	}
}

func TestValidateSectionsMinimalOK(t *testing.T) {
	req := &report.PageRequest{
		Title:  "Q3 工单与 SLA 分析",
		Format: "sections",
		Datasets: map[string]report.Dataset{
			"tickets": {
				Columns: []string{"id", "status"},
				Rows:    [][]any{{"T1", "open"}},
			},
		},
		Sections: []report.Section{
			{Type: "markdown", Content: "## 结论"},
			{
				Type: "echarts",
				ID:   "chart_custom",
				Option: map[string]any{
					"series": []any{map[string]any{"type": "line", "data": []any{1, 2}}},
				},
			},
		},
	}
	if err := report.Validate(req); err != nil {
		t.Fatalf("expected valid sections request, got: %v", err)
	}
}

func TestValidateHTMLMinimalOK(t *testing.T) {
	req := &report.PageRequest{
		Format: "html",
		HTML:   "<!DOCTYPE html><html><body><p>ok</p></body></html>",
	}
	if err := report.Validate(req); err != nil {
		t.Fatalf("expected valid html request, got: %v", err)
	}
}
