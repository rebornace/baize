package loginentry_test

import (
	"testing"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/loginentry"
	"github.com/rebornace/baize/internal/store"
)

func TestListMatchesCaptureEnabledOpenAPIHTTP(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID: "crm", Type: "openapi",
		Auth: store.ConnectorAuth{Capture: store.CaptureAuth{ToolNameGlob: "*login*"}},
	})
	st.UpsertConnector(store.Connector{ID: "side", Type: "http"})
	st.UpsertConnector(store.Connector{ID: "mcp1", Type: "mcp"})
	st.ReplaceConnectorTools("crm", []store.Tool{
		{ConnectorID: "crm", Name: "user_login", Enabled: true, InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"username": map[string]any{"type": "string"},
				"password": map[string]any{"type": "string"},
			},
			"required": []any{"username", "password"},
		}},
		{ConnectorID: "crm", Name: "list", Enabled: true},
		{ConnectorID: "crm", Name: "old_login", Enabled: false},
	})
	st.ReplaceConnectorTools("side", []store.Tool{
		{ConnectorID: "side", Name: "plugin_login", Enabled: true},
	})
	st.ReplaceConnectorTools("mcp1", []store.Tool{
		{ConnectorID: "mcp1", Name: "mcp_login", Enabled: true},
	})

	ids := identity.NewMemoryStore()
	entries := loginentry.List(st, ids, "conv1", "")
	if len(entries) != 2 {
		t.Fatalf("len=%d want 2 (crm user_login + side plugin_login via defaults)", len(entries))
	}
	if entries[0].ID != "crm/user_login" || len(entries[0].Required) != 2 {
		t.Fatalf("first=%+v", entries[0])
	}
	if entries[0].LoggedIn {
		t.Fatal("no identity => logged_in false")
	}
}

func TestListFilterConnectorAndLoggedIn(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "crm", Type: "openapi"})
	st.ReplaceConnectorTools("crm", []store.Tool{
		{ConnectorID: "crm", Name: "login", Enabled: true},
	})
	ids := identity.NewMemoryStore()
	_, _ = ids.Upsert("conv1", identity.Identity{
		Label: "u", Scheme: "Bearer", Source: identity.SourceLoginCapture,
		CredentialHeaders: map[string]string{"Authorization": "Bearer t"},
		IsDefault: true,
	})
	all := loginentry.List(st, ids, "conv1", "crm")
	if len(all) != 1 || !all[0].LoggedIn {
		t.Fatalf("%+v", all)
	}
	empty := loginentry.List(st, ids, "conv1", "missing")
	if len(empty) != 0 {
		t.Fatalf("missing connector => [] got %d", len(empty))
	}
}

func TestListMatchesPascalCaseLogin(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"}) // defaults *login*
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "AuthController_phoneLogin", Enabled: true},
		{ConnectorID: "auth", Name: "AuthController_phonePasswordLogin", Enabled: true},
		{ConnectorID: "auth", Name: "AuthController_sendSms", Enabled: true},
	})
	entries := loginentry.List(st, identity.NewMemoryStore(), "c1", "")
	if len(entries) != 2 {
		t.Fatalf("len=%d want 2 phone*Login tools, got %+v", len(entries), entries)
	}
}
