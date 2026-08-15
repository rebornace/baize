package httpplugin

import (
	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
)

// RegisterOpts configures HTTP sidecar plugin registration.
type RegisterOpts struct {
	ID              string
	BaseURL         string
	RequireApproval []string
	RequireLogin    []string // tool names that require session login in conversation
	Headers         map[string]string
	AuthMode        string
	Auth            store.ConnectorAuth
	Identities      identity.Store
	Resolver        authresolve.Resolver
}
