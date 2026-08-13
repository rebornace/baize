package openapi

import (
	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
)

// RegisterOpts configures OpenAPI connector registration.
type RegisterOpts struct {
	ID                      string
	Type                    string
	SpecPath                string
	BaseURL                 string
	RequireApproval         []string
	RequireApprovalMutating bool // if true, all non-GET/HEAD/OPTIONS tools require HITL
	Headers                 map[string]string
	AuthMode                string // "passthrough" → use ctx passthrough headers as DefaultHeaders
	Auth                    store.ConnectorAuth // stored on Connector (mode + references, not secrets)
	Identities              identity.Store         // nil → same as Headers-only behavior
	Resolver                authresolve.Resolver   // nil → use Headers only
	Capture                 identity.CaptureConfig // ToolNameGlob empty disables capture
}
