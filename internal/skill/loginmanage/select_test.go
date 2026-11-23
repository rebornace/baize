package loginmanage_test

import (
	"reflect"
	"testing"

	"github.com/rebornace/baize/internal/skill/loginmanage"
	"github.com/rebornace/baize/internal/store"
)

func TestSelectToolsCaptureAndCompanion(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "AuthController_phoneLogin", Enabled: true},
		{ConnectorID: "auth", Name: "AuthController_sendSms", Enabled: true},
		{ConnectorID: "auth", Name: "AuthController_logout", Enabled: true},
		{ConnectorID: "auth", Name: "listThings", Enabled: true},
		{ConnectorID: "auth", Name: "old_login", Enabled: false},
	})
	got := loginmanage.SelectTools(st, "auth")
	want := []string{"AuthController_phoneLogin", "AuthController_sendSms"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SelectTools=%v want %v", got, want)
	}
}

func TestSelectToolsExcludesLogoutEvenWhenCompanionAuth(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "x", Type: "http"})
	st.ReplaceConnectorTools("x", []store.Tool{
		{ConnectorID: "x", Name: "UserAuth_logout", Enabled: true},
	})
	got := loginmanage.SelectTools(st, "x")
	if len(got) != 0 {
		t.Fatalf("logout tool must be excluded, got %v", got)
	}
}

func TestSkillID(t *testing.T) {
	if loginmanage.SkillID("crm") != "login-crm" {
		t.Fatal("SkillID(crm)")
	}
	norm := loginmanage.NormalizeConnectorID("a/b")
	if norm == "" {
		t.Fatal("NormalizeConnectorID(a/b) should be non-empty")
	}
	if loginmanage.SkillID("a/b") != "login-"+norm {
		t.Fatalf("SkillID(a/b)=%q want login-%s", loginmanage.SkillID("a/b"), norm)
	}
	if loginmanage.NormalizeConnectorID("///") != "" || loginmanage.SkillID("///") != "" {
		t.Fatal("all stripped chars => empty SkillID")
	}
}
