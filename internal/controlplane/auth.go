package controlplane

import (
	"context"
	"crypto/subtle"
	"strings"
)

type Role string

const (
	RoleNone     Role = ""
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

type Tokens struct {
	Operator string
	Admin    string
}

func (t Tokens) Enabled() bool {
	return t.Operator != "" || t.Admin != ""
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
	if !strings.HasPrefix(authorizationHeader, bearerPrefix) {
		return RoleNone, false
	}
	token := authorizationHeader[len(bearerPrefix):]
	if t.Admin != "" && secretEqual(token, t.Admin) {
		return RoleAdmin, true
	}
	if t.Operator != "" && secretEqual(token, t.Operator) {
		return RoleOperator, true
	}
	return RoleNone, false
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

func WithRole(ctx context.Context, role Role) context.Context {
	return context.WithValue(ctx, roleKey{}, role)
}

func RoleFrom(ctx context.Context) Role {
	role, _ := ctx.Value(roleKey{}).(Role)
	return role
}
