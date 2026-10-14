package store

import "testing"

func TestHostedLoginMethodEffectiveRole(t *testing.T) {
	if got := (HostedLoginMethod{ID: "pwd", Role: "admin"}).EffectiveRole(); got != "admin" {
		t.Fatalf("got %q", got)
	}
	if got := (HostedLoginMethod{ID: "pwd"}).EffectiveRole(); got != "pwd" {
		t.Fatalf("got %q", got)
	}
}

func TestHostedLoginMethodIdentityBindings(t *testing.T) {
	m := HostedLoginMethod{ID: "pwd", IdentityIDs: []string{"", "mei_u", "mei_a"}}
	if got := m.PrimaryIdentityID(); got != "mei_u" {
		t.Fatalf("primary=%q", got)
	}
	if !m.BindsIdentity("mei_a") || m.BindsIdentity("mei_x") {
		t.Fatalf("binds mismatch: %+v", m.IdentityIDs)
	}
	open := HostedLoginMethod{ID: "legacy"}
	if !open.BindsIdentity("mei_any") {
		t.Fatal("empty identity_ids should be unrestricted")
	}
}

func TestHostedLoginConfigRoleForLoginTool(t *testing.T) {
	cfg := HostedLoginConfig{
		Methods: []HostedLoginMethod{
			{ID: "pwd", Role: "user", LoginTool: "mall.login"},
			{ID: "admin", LoginTool: "mall.admin_login"},
		},
	}
	if got := cfg.RoleForLoginTool("mall.login"); got != "user" {
		t.Fatalf("got %q", got)
	}
	if got := cfg.RoleForLoginTool("mall.admin_login"); got != "admin" {
		t.Fatalf("got %q", got)
	}
	if got := cfg.RoleForLoginTool("other"); got != "" {
		t.Fatalf("got %q", got)
	}
}
