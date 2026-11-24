package attach

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// extractHTML renders readable text from an HTML document, dropping scripts,
// styles, and other non-content elements.
func extractHTML(data []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	var walk func(n *html.Node, skip bool)
	walk = func(n *html.Node, skip bool) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "template", "head":
				skip = true
			}
			if !skip && isHTMLBlock(n.Data) {
				b.WriteByte('\n')
				if n.Data == "li" {
					b.WriteString("- ")
				}
			}
		}
		if n.Type == html.TextNode && !skip {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, skip)
		}
		if n.Type == html.ElementNode && !skip && isHTMLBlock(n.Data) {
			b.WriteByte('\n')
		}
	}
	walk(doc, false)
	return collapseHTMLText(b.String()), nil
}

func isHTMLBlock(tag string) bool {
	switch tag {
	case "p", "div", "section", "article", "header", "footer", "main", "aside",
		"h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "table", "tr",
		"td", "th", "blockquote", "pre", "br", "hr", "figure", "figcaption":
		return true
	}
	return false
}

func collapseHTMLText(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
