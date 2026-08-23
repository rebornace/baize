package mcp

import (
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolResultContent maps an MCP CallToolResult to Baize tool content.
func ToolResultContent(result *mcp.CallToolResult) map[string]any {
	if result == nil {
		return map[string]any{}
	}
	if result.StructuredContent != nil {
		switch v := result.StructuredContent.(type) {
		case map[string]any:
			return v
		default:
			raw, err := json.Marshal(v)
			if err != nil {
				return map[string]any{"result": v}
			}
			var out map[string]any
			if json.Unmarshal(raw, &out) == nil {
				return out
			}
			return map[string]any{"result": v}
		}
	}
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			var out map[string]any
			if json.Unmarshal([]byte(tc.Text), &out) == nil {
				return out
			}
			return map[string]any{"text": tc.Text}
		}
	}
	return map[string]any{}
}
