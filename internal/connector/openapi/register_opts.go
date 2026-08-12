package openapi

import (
	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/identity"
)

// RegisterOpts configures OpenAPI connector registration.
type RegisterOpts struct {
	ID              string
	Type            string
	SpecPath        string
	BaseURL         string
	RequireApproval []string
	Headers         map[string]string
	Identities      identity.Store         // nil → same as Headers-only behavior
	Resolver        authresolve.Resolver   // nil → use Headers only
	Capture         identity.CaptureConfig // ToolNameGlob empty disables capture
}
