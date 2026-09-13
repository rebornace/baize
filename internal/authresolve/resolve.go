package authresolve

import (
	"context"

	"github.com/rebornace/baize/internal/identity"
)

type Resolver interface {
	Resolve(ctx context.Context, in ResolveInput) Result
}

type ResolveInput struct {
	Identities      []identity.Identity
	SecuritySchemes []string
	DefaultHeaders  map[string]string
	ForceIdentityID string
}

type Result struct {
	Headers    map[string]string
	IdentityID string
	OK         bool
}
