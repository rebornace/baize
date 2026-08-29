package controlplane

import "testing"

func TestAuthenticateNamedOperators(t *testing.T) {
	tok := Tokens{
		Admin: "adm",
		Operators: []Operator{{ID: "alice", Token: "ta"}, {ID: "bob", Token: "tb"}},
	}
	p, ok := AuthenticatePrincipal("Bearer ta", tok)
	if !ok || p.Role != RoleOperator || p.OperatorID != "alice" {
		t.Fatalf("%+v ok=%v", p, ok)
	}
	p, ok = AuthenticatePrincipal("Bearer adm", tok)
	if !ok || p.Role != RoleAdmin {
		t.Fatalf("%+v", p)
	}
}

func TestAuthenticateLegacyOperatorToken(t *testing.T) {
	tok := Tokens{Operator: "op", Admin: "adm"}
	p, ok := AuthenticatePrincipal("Bearer op", tok)
	if !ok || p.OperatorID != "operator" {
		t.Fatalf("%+v", p)
	}
}
