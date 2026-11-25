package attach

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpumodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
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

const maxDocumentImages = 20
const maxDocumentImageBytes = 20 << 20

// extractPDFImages pulls embedded raster images from a PDF that has no text
// layer. The common scanned-PDF shape is one full-page image per page; vector
// or mixed scans may still yield no images and keep the usual empty-PDF error.
func extractPDFImages(data []byte, maxEdge int) []Extracted {
	conf := pdfcpumodel.NewDefaultConfiguration()
	conf.UnsupportedResourcePolicy = pdfcpumodel.UnsupportedResourceSkip
	var out []Extracted
	total := 0
	_ = pdfcpuapi.ExtractImages(context.Background(), bytes.NewReader(data), nil,
		func(img pdfcpumodel.Image, _ bool, _ int) error {
			if len(out) >= maxDocumentImages || total >= maxDocumentImageBytes || img.Reader == nil {
				return nil
			}
			if img.Thumb || img.IsImgMask {
				return nil
			}
			b, rerr := io.ReadAll(io.LimitReader(img.Reader, int64(maxDocumentImageBytes-total)))
			if rerr != nil || len(b) == 0 {
				return nil
			}
			mime := http.DetectContentType(b)
			if !isImageMIME(mime) {
				return nil
			}
			mime, thumb, perr := processImage(b, mime, maxEdge)
			if perr != nil || len(thumb) == 0 {
				return nil
			}
			total += len(thumb)
			name := img.Name
			if name == "" {
				name = fmt.Sprintf("page-%d", img.PageNr)
			}
			out = append(out, Extracted{
				Filename: name, Kind: "image",
				ImageMIME: mime, ImageBytes: thumb, Document: true,
			})
			return nil
		}, conf)
	return out
}
