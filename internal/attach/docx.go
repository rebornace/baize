package attach

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"
)

// extractDocx reads word/document.xml from a docx (Office Open XML Word)
// package and strips XML tags to recover plain text.
func extractDocx(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		raw, err := io.ReadAll(rc)
		if err != nil {
			return "", err
		}
		return stripXMLTags(string(raw)), nil
	}
	return "", errors.New("attach: docx missing word/document.xml")
}

// stripXMLTags removes XML markup and unescapes the common entities. It does
// not insert whitespace for block boundaries; callers that need paragraph
// separation should rely on the document's own spacing characters.
func stripXMLTags(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := b.String()
	out = strings.ReplaceAll(out, "&amp;", "&")
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&gt;", ">")
	out = strings.ReplaceAll(out, "&quot;", "\"")
	out = strings.ReplaceAll(out, "&apos;", "'")
	return strings.TrimSpace(out)
}
