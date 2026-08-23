package analysis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/report"
	"github.com/rebornace/baize/internal/tool"
)

const ToolName = "create_analysis_page"

// ToolSpec returns the LLM tool specification for create_analysis_page.
func ToolSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: ToolName,
		Description: "Create an interactive analysis page with charts, tables, KPIs, filters, drilldown, and PDF export. " +
			"Use format sections (recommended) with datasets, filters, and sections, or format html for a fully custom page. " +
			"Returns an artifact_url for iframe preview in chat.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "Page title shown in the header",
				},
				"format": map[string]any{
					"type":        "string",
					"enum":        []string{report.FormatSections, report.FormatHTML},
					"description": "sections (datasets + filters + sections) or html (raw HTML escape hatch)",
				},
				"theme": map[string]any{
					"type":        "string",
					"description": "Optional theme: light or dark",
				},
				"datasets": map[string]any{
					"type":        "object",
					"description": "Named tabular datasets for sections format",
					"additionalProperties": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"columns": map[string]any{
								"type":  "array",
								"items": map[string]any{"type": "string"},
							},
							"rows": map[string]any{
								"type":  "array",
								"items": map[string]any{"type": "array"},
							},
						},
					},
				},
				"filters": map[string]any{
					"type":        "array",
					"description": "Page-level filter controls",
					"items": map[string]any{
						"type": "object",
					},
				},
				"sections": map[string]any{
					"type":        "array",
					"description": "Content blocks: markdown, kpi, table, echarts, row",
					"items": map[string]any{
						"type": "object",
					},
				},
				"html": map[string]any{
					"type":        "string",
					"description": "Full HTML document or fragment for format html (max 512 KiB)",
				},
			},
		},
	}
}

// Invoker returns a tool invoker that builds analysis pages and stores them as artifacts.
func Invoker(art artifact.Store) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		req, err := decodePageRequest(args)
		if err != nil {
			return map[string]any{"error": fmt.Sprintf("invalid arguments: %v", err)}, true, nil
		}

		if err := report.Validate(req); err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}

		html, err := report.Build(req)
		if err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}

		runID := identity.RunIDFrom(ctx)
		id, err := art.PutHTML(runID, html)
		if err != nil {
			return nil, false, fmt.Errorf("store artifact: %w", err)
		}

		format := req.Format
		if format == "" {
			format = report.FormatSections
		}

		return map[string]any{
			"artifact_id":   id,
			"artifact_url":  "/v0/artifacts/" + id,
			"kind":          "analysis_page",
			"format":        format,
			"section_count": len(req.Sections),
		}, false, nil
	}
}

func decodePageRequest(args map[string]any) (*report.PageRequest, error) {
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	var req report.PageRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	return &req, nil
}
