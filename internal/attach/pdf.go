package attach

import (
	"bytes"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// extractPDF pulls the text layer from a PDF. An empty result is reported by
// the caller as ErrEmptyPDFText.
func extractPDF(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	pr, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	raw, err := io.ReadAll(pr)
	if err != nil {
		return "", err
	}
	return normalizePDFText(string(raw)), nil
}

// normalizePDFText collapses the whitespace that PDF text streams frequently
// emit between glyphs/words into single spaces.
func normalizePDFText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	started := false
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if size == 0 {
			break
		}
		isSpace := r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v'
		if isSpace {
			prevSpace = true
		} else {
			if prevSpace && started {
				b.WriteByte(' ')
			}
			b.WriteString(s[i : i+size])
			prevSpace = false
			started = true
		}
		i += size
	}
	return strings.TrimSpace(b.String())
}
