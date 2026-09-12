package tool

import (
	"testing"

	"github.com/rebornace/baize/internal/llm"
)

func TestWithAndExtractImageParts(t *testing.T) {
	content := map[string]any{"path": "uploads/x.png", "bytes": 3}
	res := ImageResult{Path: "uploads/x.png", Part: llm.ContentPart{Type: "image", ImageMIME: "image/png", ImageBytes: []byte("PNGDATA")}}

	with := WithImageParts(content, res)
	if with["path"] != "uploads/x.png" {
		t.Fatalf("original keys must be preserved: %v", with)
	}

	cleaned, results := ExtractImageParts(with)
	if len(results) != 1 || results[0].Path != "uploads/x.png" {
		t.Fatalf("want 1 image result with path, got %v", results)
	}
	if results[0].Part.ImageMIME != "image/png" || string(results[0].Part.ImageBytes) != "PNGDATA" {
		t.Fatalf("image part lost: %+v", results[0])
	}
	if _, leak := cleaned[imagePartsKey]; leak {
		t.Fatalf("reserved key must be stripped from cleaned content: %v", cleaned)
	}
	if cleaned["path"] != "uploads/x.png" {
		t.Fatalf("cleaned content must keep text keys: %v", cleaned)
	}
}

func TestExtractImagePartsNone(t *testing.T) {
	content := map[string]any{"ok": true}
	cleaned, results := ExtractImageParts(content)
	if len(results) != 0 {
		t.Fatalf("want no results, got %v", results)
	}
	if cleaned["ok"] != true {
		t.Fatalf("content must be returned intact: %v", cleaned)
	}
}
