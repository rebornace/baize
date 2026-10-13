package mcpexport

import (
	"strings"

	"github.com/rebornace/baize/internal/store"
)

// PolicyOpts carries connector-level options that affect export decisions.
type PolicyOpts struct {
	DBReadonlyConnector bool
}

// AllowExportToolGuards applies catalog-level export rules (enabled, export mode,
// method heuristic, force_allow). It does not apply per-key allowlists.
func AllowExportToolGuards(tool store.Tool, opts PolicyOpts) bool {
	if !tool.Enabled {
		return false
	}

	export := tool.Export
	if export == "" {
		export = "default"
	}
	if export == "force_deny" {
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

// AllowExportForKey intersects AllowExportToolGuards with the key allowlist.
// Empty AllowedTools is fail-closed (no tools).
func AllowExportForKey(tool store.Tool, opts PolicyOpts, key store.MCPExportKey) bool {
	if len(key.AllowedTools) == 0 {
		return false
	}
	if !toolAllowedByKey(tool.Name, key.AllowedTools) {
		return false
	}
	return AllowExportToolGuards(tool, opts)
}

func toolAllowedByKey(name string, allowed []string) bool {
	for _, n := range allowed {
		if n == name {
			return true
		}
	}
	return false
}

// AllowExport is a thin wrapper over AllowExportToolGuards for callers that do
// not have a key context (e.g. one-shot allowlist backfill migration).
func AllowExport(tool store.Tool, opts PolicyOpts) bool {
	return AllowExportToolGuards(tool, opts)
}

func methodAllowsExport(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "HEAD":
		return true
	default:
		return false
	}
}
