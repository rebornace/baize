package mcp

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rebornace/baize/internal/store"
)

// AnnotationHints copies MCP tool annotation hints into store.Tool pointer fields.
func AnnotationHints(ann *mcp.ToolAnnotations) (readOnly, destructive *bool) {
	if ann == nil {
		return nil, nil
	}
	if ann.ReadOnlyHint {
		v := true
		readOnly = &v
	}
	if ann.DestructiveHint != nil {
		v := *ann.DestructiveHint
		destructive = &v
	}
	return readOnly, destructive
}

// ApplyAnnotationHints sets ReadOnlyHint and DestructiveHint on dst from MCP annotations.
func ApplyAnnotationHints(dst *store.Tool, ann *mcp.ToolAnnotations) {
	if dst == nil {
		return
	}
	dst.ReadOnlyHint, dst.DestructiveHint = AnnotationHints(ann)
}
