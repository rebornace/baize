package mcpexport_test

import (
	"testing"

	"github.com/rebornace/baize/internal/mcpexport"
	"github.com/rebornace/baize/internal/store"
)

func TestAllowExportToolGuardsMatrix(t *testing.T) {
	cases := []struct {
		name string
		tool store.Tool
		dbRO bool // connector export_db_readonly
		want bool
	}{
		{"get_default", store.Tool{Enabled: true, Method: "GET", Export: "default"}, false, true},
		{"head_default", store.Tool{Enabled: true, Method: "HEAD", Export: "default"}, false, true},
		{"post_default", store.Tool{Enabled: true, Method: "POST", Export: "default"}, false, false},
		{"post_force", store.Tool{Enabled: true, Method: "POST", Export: "force_allow"}, false, true},
		{"get_deny", store.Tool{Enabled: true, Method: "GET", Export: "force_deny"}, false, false},
		{"empty_method", store.Tool{Enabled: true, Method: "", Export: "default"}, false, false},
		{"empty_force", store.Tool{Enabled: true, Method: "", Export: "force_allow"}, false, true},
		{"disabled", store.Tool{Enabled: false, Method: "GET", Export: "default"}, false, false},
		{"approval", store.Tool{Enabled: true, Method: "GET", RequireApproval: true, Export: "default"}, false, true},
		{"mcp_write_force", store.Tool{Enabled: true, Source: store.ToolSourceMCP, Name: "insert_row", Description: "insert into t", Export: "force_allow"}, true, true},
		{"mcp_write_force_no_dbro", store.Tool{Enabled: true, Source: store.ToolSourceMCP, Name: "insert_row", Description: "insert into t", Export: "force_allow"}, false, true},
		{"mcp_query", store.Tool{Enabled: true, Source: store.ToolSourceMCP, Name: "query", Description: "run select", Export: "default"}, true, true},
	}
	opts := func(dbRO bool) mcpexport.PolicyOpts {
		return mcpexport.PolicyOpts{DBReadonlyConnector: dbRO}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mcpexport.AllowExportToolGuards(tc.tool, opts(tc.dbRO))
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestAllowExportForKeyAllowlist(t *testing.T) {
	tool := store.Tool{Enabled: true, Method: "GET", Export: "default", Name: "get_status"}
	opts := mcpexport.PolicyOpts{}
	keyEmpty := store.MCPExportKey{}
	keyMissing := store.MCPExportKey{AllowedTools: []string{"other"}}
	keyOK := store.MCPExportKey{AllowedTools: []string{"get_status"}}
	keyDenyListed := store.MCPExportKey{AllowedTools: []string{"get_status"}}
	denyTool := store.Tool{Enabled: true, Method: "GET", Export: "force_deny", Name: "get_status"}

	if mcpexport.AllowExportForKey(tool, opts, keyEmpty) {
		t.Fatal("empty allowlist should fail closed")
	}
	if mcpexport.AllowExportForKey(tool, opts, keyMissing) {
		t.Fatal("tool not in allowlist")
	}
	if !mcpexport.AllowExportForKey(tool, opts, keyOK) {
		t.Fatal("expected allowed")
	}
	if mcpexport.AllowExportForKey(denyTool, opts, keyDenyListed) {
		t.Fatal("force_deny guard should win over allowlist")
	}
}

func TestAllowExportForKeyReadOnlyHintDoesNotBypassEmptyAllowlist(t *testing.T) {
	readOnly := true
	tool := store.Tool{
		Enabled:      true,
		Method:       "GET",
		Export:       "default",
		Name:         "read_only_tool",
		ReadOnlyHint: &readOnly,
	}
	opts := mcpexport.PolicyOpts{}
	keyEmpty := store.MCPExportKey{}
	if mcpexport.AllowExportForKey(tool, opts, keyEmpty) {
		t.Fatal("ReadOnlyHint must not bypass empty allowlist")
	}
}
