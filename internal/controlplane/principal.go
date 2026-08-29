package controlplane

import "strings"

type Operator struct {
	ID    string
	Token string
}

type Principal struct {
	Role       Role
	OperatorID string
}

func AuthenticatePrincipal(authorizationHeader string, t Tokens) (Principal, bool) {
	if !strings.HasPrefix(authorizationHeader, bearerPrefix) {
		return Principal{}, false
	}
	token := authorizationHeader[len(bearerPrefix):]
	if t.Admin != "" && secretEqual(token, t.Admin) {
		return Principal{Role: RoleAdmin}, true
	}
	if t.Operator != "" && secretEqual(token, t.Operator) {
		return Principal{Role: RoleOperator, OperatorID: "operator"}, true
	}
	for _, op := range t.Operators {
		if op.Token != "" && secretEqual(token, op.Token) {
			return Principal{Role: RoleOperator, OperatorID: op.ID}, true
		}
	}
	return Principal{}, false
}
