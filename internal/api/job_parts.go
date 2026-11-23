package api

import (
	"encoding/base64"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/middleware"
)

// PartsToMiddleware converts LLM multimodal content parts into queue-safe
// middleware parts. Image bytes are encoded as a base64 data: URI (mirroring
// internal/llm openai content encoding); a pre-built DataURL is forwarded
// verbatim. Image parts with neither bytes nor a DataURL are dropped to avoid
// emitting a malformed image entry.
func PartsToMiddleware(parts []llm.ContentPart) []middleware.Part {
	out := make([]middleware.Part, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			out = append(out, middleware.Part{Type: "text", Text: p.Text})
		case "image":
			url := p.DataURL
			if url == "" {
				if len(p.ImageBytes) == 0 {
					continue
				}
				mime := p.ImageMIME
				if mime == "" {
					mime = "image/png"
				}
				url = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(p.ImageBytes)
			}
			out = append(out, middleware.Part{Type: "image", DataURL: url})
		}
	}
	return out
}

// partsFromMiddleware converts queued parts back into LLM content parts.
// Images ride as data: URIs, which providers forward verbatim.
func partsFromMiddleware(parts []middleware.Part) []llm.ContentPart {
	out := make([]llm.ContentPart, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			out = append(out, llm.ContentPart{Type: "text", Text: p.Text})
		case "image":
			out = append(out, llm.ContentPart{Type: "image", DataURL: p.DataURL})
		}
	}
	return out
}
