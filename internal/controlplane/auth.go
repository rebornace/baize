package controlplane

import (
	"context"
	"crypto/subtle"
)

type Role string

const (
	RoleNone     Role = ""
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

type Tokens struct {
	Operator  string
	Admin     string
	Operators []Operator
}

func (t Tokens) Enabled() bool {
	if t.Admin != "" || t.Operator != "" {
		return true
	}
	for _, op := range t.Operators {
		if op.Token != "" {
			return true
		}
	}
	return false
}

func (r Role) AtLeast(min Role) bool {
	switch min {
	case RoleNone:
		return true
	case RoleOperator:
		return r == RoleOperator || r == RoleAdmin
	case RoleAdmin:
		return r == RoleAdmin
	default:
		return false
	}
}

const bearerPrefix = "Bearer "

func Authenticate(authorizationHeader string, t Tokens) (Role, bool) {
	p, ok := AuthenticatePrincipal(authorizationHeader, t)
	return p.Role, ok
}

func secretEqual(got, want string) bool {
	if want == "" {
		return false
	}
	g := []byte(got)
	w := []byte(want)
	if len(g) != len(w) {
		padded := make([]byte, len(w))
		copy(padded, g)
		subtle.ConstantTimeCompare(padded, w)
		return false
	}
	return subtle.ConstantTimeCompare(g, w) == 1
}

type roleKey struct{}

type operatorIDKey struct{}

func WithRole(ctx context.Context, role Role) context.Context {
	return context.WithValue(ctx, roleKey{}, role)
}

func RoleFrom(ctx context.Context) Role {
	role, _ := ctx.Value(roleKey{}).(Role)
	return role
}

func WithOperatorID(ctx context.Context, operatorID string) context.Context {
	return context.WithValue(ctx, operatorIDKey{}, operatorID)
}

func OperatorIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(operatorIDKey{}).(string)
	return id
}
