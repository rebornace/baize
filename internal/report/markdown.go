package report

import (
	"bytes"
	"html"
	"strings"

	"github.com/yuin/goldmark"
)

var markdownRenderer = goldmark.New()

// RenderMarkdown converts markdown to safe HTML without raw script tags.
func RenderMarkdown(source string) (string, error) {
	var buf bytes.Buffer
	if err := markdownRenderer.Convert([]byte(source), &buf); err != nil {
		return "", err
	}
	out := buf.String()
	if strings.Contains(strings.ToLower(out), "<script") {
		return html.EscapeString(source), nil
	}
	return out, nil
}
