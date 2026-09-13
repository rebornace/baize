package mcpexport

import (
	"strings"

	"github.com/rebornace/baize/internal/store"
)

// PolicyOpts carries connector-level options that affect export decisions.
type PolicyOpts struct {
	DBReadonlyConnector bool
}

// AllowExport decides whether a catalog tool may appear on the MCP export surface.
func AllowExport(tool store.Tool, opts PolicyOpts) bool {
	if !tool.Enabled {
		return false
	}
	if tool.RequireApproval {
		return false
	}

	export := tool.Export
	if export == "" {
		export = "default"
	}
	if export == "force_deny" {
		return false
	}

	// Spec §3.4: MCP write tools are never exported, even with force_allow.
	if tool.Source == store.ToolSourceMCP && IsMCPWriteTool(tool.Name, tool.Description) {
		return false
	}

	if tool.Source == store.ToolSourceMCP && opts.DBReadonlyConnector {
		if export == "force_allow" {
			return true
		}
		if tool.Method == "" {
			return true
		}
		return methodAllowsExport(tool.Method)
	}

	if export == "force_allow" {
		return true
	}

	return methodAllowsExport(tool.Method)
}

func methodAllowsExport(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "HEAD":
		return true
	default:
		return false
	}
}
