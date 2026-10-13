package mcp_test

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
)

func TestAnnotationHints(t *testing.T) {
	destructive := true
	ro, dh := mcpbridge.AnnotationHints(&mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: &destructive,
	})
	if ro == nil || !*ro {
		t.Fatalf("readOnly=%v", ro)
	}
	if dh == nil || !*dh {
		t.Fatalf("destructive=%v", dh)
	}

	ro2, dh2 := mcpbridge.AnnotationHints(nil)
	if ro2 != nil || dh2 != nil {
		t.Fatalf("nil annotations should yield nil hints")
	}
}
