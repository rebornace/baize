package report

import (
	"encoding/json"
	"fmt"
	"strings"
)

type pagePayload struct {
	Title    string             `json:"title,omitempty"`
	Theme    string             `json:"theme,omitempty"`
	Datasets map[string]Dataset `json:"datasets,omitempty"`
	Filters  []Filter           `json:"filters,omitempty"`
	Sections []Section          `json:"sections,omitempty"`
}

// Build renders a PageRequest into a self-contained HTML document.
func Build(req *PageRequest) (string, error) {
	if err := Validate(req); err != nil {
		return "", err
	}

	format := strings.TrimSpace(req.Format)
	if format == "" {
		format = FormatSections
	}

	switch format {
	case FormatSections:
		return buildSections(req)
	case FormatHTML:
		return WrapHTML(req.HTML), nil
	default:
		return "", fmt.Errorf("unknown format %q", req.Format)
	}
}

func buildSections(req *PageRequest) (string, error) {
	payload := pagePayload{
		Title:    req.Title,
		Theme:    req.Theme,
		Datasets: req.Datasets,
		Filters:  req.Filters,
		Sections: req.Sections,
	}

	pageJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal page json: %w", err)
	}

	sectionsHTML, err := renderSectionPlaceholders(req.Sections)
	if err != nil {
		return "", err
	}

	title := req.Title
	if title == "" {
		title = "Analysis"
	}

	html := shellHTML
	html = strings.ReplaceAll(html, "__BAIZE_TITLE__", escapeHTML(title))
	html = strings.ReplaceAll(html, "__BAIZE_SECTIONS__", sectionsHTML)
	html = strings.ReplaceAll(html, "__BAIZE_PAGE_JSON__", string(pageJSON))
	html = strings.ReplaceAll(html, "__BAIZE_ECHARTS_JS__", echartsJS)
	html = strings.ReplaceAll(html, "__BAIZE_RUNTIME_JS__", runtimeJS)
	return html, nil
}

func renderSectionPlaceholders(sections []Section) (string, error) {
	var b strings.Builder
	for i, sec := range sections {
		fmt.Fprintf(&b, `<div class="baize-section" data-section-index="%d" data-section-type="%s">`, i, escapeAttr(sec.Type))
		switch sec.Type {
		case "markdown":
			body, err := RenderMarkdown(sec.Content)
			if err != nil {
				return "", fmt.Errorf("sections[%d]: render markdown: %w", i, err)
			}
			fmt.Fprintf(&b, `<div class="markdown-body">%s</div>`, body)
		}
		b.WriteString("</div>\n")
	}
	return b.String(), nil
}

const pdfButtonHTML = `<button id="export-pdf" type="button" onclick="window.print()">导出 PDF</button>`

const pdfButtonScript = `<script>(function(){var b=document.getElementById("export-pdf");if(b){b.addEventListener("click",function(){window.print();});}})();</script>`

// WrapHTML wraps fragment HTML in a document and injects a PDF export control when missing.
func WrapHTML(html string) string {
	trimmed := strings.TrimSpace(html)
	if trimmed == "" {
		trimmed = "<body></body>"
	}

	lower := strings.ToLower(trimmed)
	if !strings.Contains(lower, "<html") {
		trimmed = "<!DOCTYPE html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"></head>" + trimmed + "</html>"
	}

	if !strings.Contains(lower, `id="export-pdf"`) && !strings.Contains(lower, "id='export-pdf'") {
		if strings.Contains(strings.ToLower(trimmed), "</body>") {
			trimmed = strings.Replace(trimmed, "</body>", pdfButtonHTML+pdfButtonScript+"</body>", 1)
		} else {
			trimmed += pdfButtonHTML + pdfButtonScript
		}
	}

	return trimmed
}

func escapeHTML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return replacer.Replace(s)
}

func escapeAttr(s string) string {
	return escapeHTML(s)
}
