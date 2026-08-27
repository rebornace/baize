// Package attach extracts text and image content from chat attachments.
//
// Supported attachment types:
//   - text: text/plain, text/markdown, text/csv
//   - documents: docx (Office Open XML Word), xlsx (Office Open XML Spreadsheet), pdf (text layer)
//   - images: png, jpeg, webp, gif (decoded, thumbnail-capped, re-encoded)
//
// All limits are enforced via Options; DefaultOptions matches the approved spec.
package attach

import (
	"encoding/base64"
	"errors"
	"fmt"
)

// AttachmentIn is a single inbound attachment as received from the API.
type AttachmentIn struct {
	Filename   string `json:"filename"`
	MediaType  string `json:"media_type"`
	ContentB64 string `json:"content_base64"`
}

// Extracted is the result of processing one attachment.
type Extracted struct {
	Filename   string
	Kind       string // "text" | "image"
	Text       string // Kind == "text"
	ImageMIME  string // Kind == "image"
	ImageBytes []byte // Kind == "image", thumbnail-capped
}

// Options controls attachment processing limits.
type Options struct {
	MaxCount      int // max number of attachments per request
	MaxTotalBytes int // max sum of decoded content bytes across all attachments
	MaxTextChars  int // max characters per extracted text
	MaxImageEdge  int // max long edge (px) for image thumbnails
}

// DefaultOptions returns the approved spec limits.
func DefaultOptions() Options {
	return Options{
		MaxCount:      5,
		MaxTotalBytes: 8 << 20,
		MaxTextChars:  64 << 10,
		MaxImageEdge:  2048,
	}
}

// Sentinel errors. ErrUnsupported maps to the API `unsupported_attachment` code.
var (
	ErrUnsupported  = errors.New("attach: unsupported attachment type")
	ErrTooLarge     = errors.New("attach: attachment too large")
	ErrTooMany      = errors.New("attach: too many attachments")
	ErrEmptyPDFText = errors.New("attach: pdf has no extractable text")
)

// Process extracts text and image attachments. It returns the text and image
// results separately; on error the returned slices are nil.
//
// Any zero-valued Options field is replaced with its DefaultOptions value, so
// a caller may pass a partially populated Options (or the zero value) and get
// the spec defaults for the fields they left unset.
func Process(atts []AttachmentIn, opts Options) (texts []Extracted, images []Extracted, err error) {
	opts = opts.withDefaults()
	if len(atts) > opts.MaxCount {
		return nil, nil, fmt.Errorf("%w: %d > %d", ErrTooMany, len(atts), opts.MaxCount)
	}

	// Pre-decode and budget-check so a too-large payload fails before heavy work.
	decoded := make([][]byte, len(atts))
	var total int64
	for i, a := range atts {
		b, derr := base64.StdEncoding.DecodeString(a.ContentB64)
		if derr != nil {
			return nil, nil, fmt.Errorf("attach: decode %s: %w", a.Filename, derr)
		}
		decoded[i] = b
		total += int64(len(b))
		if total > int64(opts.MaxTotalBytes) {
			return nil, nil, fmt.Errorf("%w: %d > %d", ErrTooLarge, total, opts.MaxTotalBytes)
		}
	}

	texts = make([]Extracted, 0, len(atts))
	images = make([]Extracted, 0, len(atts))
	for i, a := range atts {
		ex := Extracted{Filename: a.Filename}
		switch {
		case isTextMIME(a.MediaType):
			ex.Kind = "text"
			ex.Text = truncateText(string(decoded[i]), opts.MaxTextChars)
			texts = append(texts, ex)
		case a.MediaType == mimeDocx:
			t, perr := extractDocx(decoded[i])
			if perr != nil {
				return nil, nil, perr
			}
			ex.Kind = "text"
			ex.Text = truncateText(t, opts.MaxTextChars)
			texts = append(texts, ex)
		case a.MediaType == mimeXlsx:
			t, perr := extractXlsx(decoded[i])
			if perr != nil {
				return nil, nil, perr
			}
			ex.Kind = "text"
			ex.Text = truncateText(t, opts.MaxTextChars)
			texts = append(texts, ex)
		case a.MediaType == mimePDF:
			t, perr := extractPDF(decoded[i])
			if perr != nil {
				return nil, nil, perr
			}
			if t == "" {
				return nil, nil, ErrEmptyPDFText
			}
			ex.Kind = "text"
			ex.Text = truncateText(t, opts.MaxTextChars)
			texts = append(texts, ex)
		case isImageMIME(a.MediaType):
			mime, thumb, perr := processImage(decoded[i], a.MediaType, opts.MaxImageEdge)
			if perr != nil {
				return nil, nil, perr
			}
			ex.Kind = "image"
			ex.ImageMIME = mime
			ex.ImageBytes = thumb
			images = append(images, ex)
		default:
			return nil, nil, fmt.Errorf("%w: %s", ErrUnsupported, a.MediaType)
		}
	}
	return texts, images, nil
}

// withDefaults returns a copy of opts where every zero-valued field is
// replaced by the corresponding DefaultOptions value. This lets callers pass
// a partially populated Options and still get sane limits for the rest.
func (o Options) withDefaults() Options {
	d := DefaultOptions()
	if o.MaxCount == 0 {
		o.MaxCount = d.MaxCount
	}
	if o.MaxTotalBytes == 0 {
		o.MaxTotalBytes = d.MaxTotalBytes
	}
	if o.MaxTextChars == 0 {
		o.MaxTextChars = d.MaxTextChars
	}
	if o.MaxImageEdge == 0 {
		o.MaxImageEdge = d.MaxImageEdge
	}
	return o
}

// truncatedMarker is appended to any text that is cut by truncateText so the
// model (and the operator reading logs) can tell the content was shortened, as
// promised by the spec and README.
const truncatedMarker = "…[truncated]"

// truncateText truncates s to at most max runes and, when truncation occurs,
// appends truncatedMarker. MaxTextChars is defined in characters (runes), so
// multi-byte text is cut on a rune boundary and never produces a partial-
// codepoint tail. The marker sits outside the rune budget so the visible
// content prefix is exactly max runes.
func truncateText(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + truncatedMarker
}

func isTextMIME(m string) bool {
	switch m {
	case "text/plain", "text/markdown", "text/csv":
		return true
	}
	return false
}

func isImageMIME(m string) bool {
	switch m {
	case "image/png", "image/jpeg", "image/jpg", "image/webp", "image/gif":
		return true
	}
	return false
}

const (
	mimeDocx = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	mimeXlsx = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	mimePDF  = "application/pdf"
)
