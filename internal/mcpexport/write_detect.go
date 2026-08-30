package mcpexport

import "strings"

// mcpWriteSignals lists substrings matched case-insensitively against tool
// name or description to classify MCP database write tools (v0 hard deny).
var mcpWriteSignals = []string{
	"insert",
	"update",
	"delete",
	"drop",
	"truncate",
	"execute_write",
}

// IsMCPWriteTool reports whether an MCP tool name or description indicates a
// database write operation. Matching is case-insensitive substring search
// against mcpWriteSignals.
func IsMCPWriteTool(name, desc string) bool {
	lowerName := strings.ToLower(name)
	lowerDesc := strings.ToLower(desc)
	for _, signal := range mcpWriteSignals {
		if strings.Contains(lowerName, signal) || strings.Contains(lowerDesc, signal) {
			return true
		}
	}
	return false
}
