package toolindex

import (
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/llm"
)

// Document is one indexed tool: searchable text plus metadata for ranking.
type Document struct {
	Name   string
	Source string
	Text   string
	Tokens map[string]bool
	// TermFreq is bag-of-tokens counts over Text (for BM25 / TF-IDF).
	TermFreq map[string]int
}

// BuildDocument indexes a ToolSpec as name + method + path + description +
// JSON-schema property names. Path segments are repeated so /users ranks for
// "users" even when the operationId is opaque.
func BuildDocument(s llm.ToolSpec) Document {
	var b strings.Builder
	b.WriteString(s.Name)
	b.WriteByte(' ')
	if m := strings.TrimSpace(s.Method); m != "" {
		b.WriteString(m)
		b.WriteByte(' ')
	}
	if p := strings.TrimSpace(s.Path); p != "" {
		b.WriteString(p)
		b.WriteByte(' ')
		// Emphasize path segments (entity nouns live here on OpenAPI tools).
		for seg := range PathTokens(p) {
			b.WriteString(seg)
			b.WriteByte(' ')
			b.WriteString(seg)
			b.WriteByte(' ')
		}
	}
	b.WriteString(firstLine(s.Description))
	b.WriteByte(' ')
	for _, prop := range schemaPropertyNames(s.InputSchema) {
		b.WriteString(prop)
		b.WriteByte(' ')
	}
	text := strings.TrimSpace(b.String())
	toks := Tokens(text)
	tf := make(map[string]int, len(toks))
	for t := range toks {
		tf[t]++
	}
	// Count path/name tokens twice in TF for BM25 emphasis.
	for t := range Tokens(s.Name + " " + s.Path) {
		tf[t]++
	}
	return Document{
		Name:     s.Name,
		Source:   s.Source,
		Text:     text,
		Tokens:   toks,
		TermFreq: tf,
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func schemaPropertyNames(schema map[string]any) []string {
	if schema == nil {
		return nil
	}
	props, _ := schema["properties"].(map[string]any)
	if len(props) == 0 {
		return nil
	}
	names := make([]string, 0, len(props))
	for k := range props {
		k = strings.TrimSpace(k)
		if k != "" {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	return names
}
