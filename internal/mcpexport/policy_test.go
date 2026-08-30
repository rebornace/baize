package mcpexport_test

import (
	"testing"

	"github.com/rebornace/baize/internal/mcpexport"
	"github.com/rebornace/baize/internal/store"
)

func TestAllowExportMatrix(t *testing.T) {
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
		{"approval", store.Tool{Enabled: true, Method: "GET", RequireApproval: true, Export: "default"}, false, false},
		{"mcp_write_force", store.Tool{Enabled: true, Source: store.ToolSourceMCP, Name: "insert_row", Description: "insert into t", Export: "force_allow"}, true, false},
		{"mcp_write_force_no_dbro", store.Tool{Enabled: true, Source: store.ToolSourceMCP, Name: "insert_row", Description: "insert into t", Export: "force_allow"}, false, false},
		{"mcp_query", store.Tool{Enabled: true, Source: store.ToolSourceMCP, Name: "query", Description: "run select", Export: "default"}, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mcpexport.AllowExport(tc.tool, mcpexport.PolicyOpts{DBReadonlyConnector: tc.dbRO})
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
