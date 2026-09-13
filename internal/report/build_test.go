package report_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/report"
)

func TestBuildSectionsContainsPageJSON(t *testing.T) {
	req := &report.PageRequest{
		Title:  "Demo",
		Format: "sections",
		Datasets: map[string]report.Dataset{
			"t": {Columns: []string{"a"}, Rows: [][]any{{"1"}}},
		},
		Sections: []report.Section{{Type: "markdown", Content: "## Hi"}},
	}
	html, err := report.Build(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "__BAIZE_PAGE__") {
		t.Fatal("missing page json marker")
	}
	if !strings.Contains(html, "echarts") {
		t.Fatal("missing echarts embed")
	}
}
