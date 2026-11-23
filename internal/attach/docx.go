package attach

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
)

// extractDocx reads word/document.xml from a docx (Office Open XML Word)
// package and strips XML tags to recover plain text.
func extractDocx(data []byte) (string, error) {
	if err := checkZipExpansion(data); err != nil {
		return "", err
	}
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
		// Header sizes are attacker-controlled; cap the actual read too.
		raw, err := io.ReadAll(io.LimitReader(rc, maxDocUncompressed+1))
		if err != nil {
			return "", err
		}
		if len(raw) > maxDocUncompressed {
			return "", fmt.Errorf("%w: document expands beyond %d bytes", ErrTooLarge, maxDocUncompressed)
		}
		return stripXMLTags(string(raw)), nil
	}
	return "", errors.New("attach: docx missing word/document.xml")
}

// checkZipExpansion rejects a zip whose declared uncompressed content exceeds
// maxDocUncompressed. Shared by docx and xlsx extraction.
func checkZipExpansion(data []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
		if total > maxDocUncompressed {
			return fmt.Errorf("%w: document expands beyond %d bytes", ErrTooLarge, maxDocUncompressed)
		}
	}
	return nil
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
