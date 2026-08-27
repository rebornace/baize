package controlplane

import "testing"

func TestAuthenticateGateOff(t *testing.T) {
	tok := Tokens{}
	if tok.Enabled() {
		t.Fatal("empty tokens must disable gate")
	}
}

func TestAuthenticateRoles(t *testing.T) {
	tok := Tokens{Operator: "op", Admin: "adm"}
	if !tok.Enabled() {
		t.Fatal("expected enabled")
	}
	role, ok := Authenticate("Bearer adm", tok)
	if !ok || role != RoleAdmin {
		t.Fatalf("admin: %q %v", role, ok)
	}
	role, ok = Authenticate("Bearer op", tok)
	if !ok || role != RoleOperator {
		t.Fatalf("operator: %q %v", role, ok)
	}
	if _, ok := Authenticate("Bearer nope", tok); ok {
		t.Fatal("bad token")
	}
	if _, ok := Authenticate("", tok); ok {
		t.Fatal("missing")
	}
	if _, ok := Authenticate("op", tok); ok {
		t.Fatal("must require Bearer prefix")
	}
}

func TestAuthenticateSameValueIsAdmin(t *testing.T) {
	tok := Tokens{Operator: "same", Admin: "same"}
	role, ok := Authenticate("Bearer same", tok)
	if !ok || role != RoleAdmin {
		t.Fatalf("got %q %v", role, ok)
	}
}

func TestAuthenticateOperatorOnly(t *testing.T) {
	tok := Tokens{Operator: "op"}
	role, ok := Authenticate("Bearer op", tok)
	if !ok || role != RoleOperator {
		t.Fatalf("got %q %v", role, ok)
	}
}

func TestRoleAtLeast(t *testing.T) {
	if !RoleAdmin.AtLeast(RoleOperator) || !RoleAdmin.AtLeast(RoleAdmin) {
		t.Fatal("admin is superset")
	}
	if RoleOperator.AtLeast(RoleAdmin) {
		t.Fatal("operator must not satisfy admin")
	}
	if !RoleOperator.AtLeast(RoleOperator) {
		t.Fatal("operator satisfies operator")
	}
}
