package mcpexport

import (
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestResolveExportIdentityPrefersConnectorSlot(t *testing.T) {
	st := store.NewMemory()
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_def", Name: "Default"})
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_mall", Name: "Mall"})
	key := store.MCPExportKey{
		IdentityID: "mei_def",
		IdentitySlots: []store.MCPExportIdentitySlot{
			{IdentityID: "mei_def"},
			{ConnectorID: "mall", IdentityID: "mei_mall"},
		},
	}
	got, err := resolveExportIdentity(st, key, "mall", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "mei_mall" || got.Name != "Mall" {
		t.Fatalf("got=%+v", got)
	}
	fallback, err := resolveExportIdentity(st, key, "other", "")
	if err != nil {
		t.Fatal(err)
	}
	if fallback.ID != "mei_def" {
		t.Fatalf("fallback=%+v", fallback)
	}
}

func TestExportRoleForToolMatchesHostedLoginTool(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID:   "mall",
		Type: "openapi",
		Auth: store.ConnectorAuth{
			HostedLogin: store.HostedLoginConfig{
				Enabled:         true,
				DefaultMethodID: "pwd",
				Methods: []store.HostedLoginMethod{
					{ID: "pwd", Role: "user", LoginTool: "mall.login"},
					{ID: "tok", LoginTool: "mall.token"},
				},
			},
		},
	})
	if got := exportRoleForTool(st, "mall", "mall.login"); got != "user" {
		t.Fatalf("login role=%q", got)
	}
	if got := exportRoleForTool(st, "mall", "mall.token"); got != "tok" {
		t.Fatalf("token role=%q", got)
	}
	if got := exportRoleForTool(st, "mall", "mall.get_orders"); got != "" {
		t.Fatalf("business tool should use empty role, got %q", got)
	}
}

func TestResolveExportIdentityLoginToolRoleSlot(t *testing.T) {
	st := store.NewMemory()
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_user", Name: "User"})
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_admin", Name: "Admin"})
	st.UpsertConnector(store.Connector{
		ID: "mall", Type: "openapi",
		Auth: store.ConnectorAuth{
			HostedLogin: store.HostedLoginConfig{
				Enabled: true, DefaultMethodID: "pwd",
				Methods: []store.HostedLoginMethod{
					{ID: "pwd", Role: "user", LoginTool: "mall.login"},
					{ID: "adm", Role: "admin", LoginTool: "mall.admin_login"},
				},
			},
		},
	})
	key := store.MCPExportKey{
		IdentityID: "mei_user",
		IdentitySlots: []store.MCPExportIdentitySlot{
			{IdentityID: "mei_user"},
			{ConnectorID: "mall", IdentityID: "mei_user"},
			{ConnectorID: "mall", Role: "admin", IdentityID: "mei_admin"},
			{ConnectorID: "mall", Role: "user", IdentityID: "mei_user"},
		},
	}
	role := exportRoleForTool(st, "mall", "mall.admin_login")
	got, err := resolveExportIdentity(st, key, "mall", role)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "mei_admin" {
		t.Fatalf("got=%+v role=%q", got, role)
	}
	bizRole := exportRoleForTool(st, "mall", "mall.get")
	biz, err := resolveExportIdentity(st, key, "mall", bizRole)
	if err != nil {
		t.Fatal(err)
	}
	if biz.ID != "mei_user" {
		t.Fatalf("business=%+v", biz)
	}
}
