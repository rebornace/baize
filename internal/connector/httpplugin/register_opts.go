package httpplugin

import (
	"time"

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
	Capture         identity.CaptureConfig // ToolNameGlob empty disables capture

	// CallbackURL injection (Phase 2). When all four are usable and the
	// per-invoke ctx carries a RunID, RegisterWithOpts issues a short-lived
	// token via CallbackSigner and advertises
	//   {CallbackPublicBase}/v0/runs/{runID}/plugin-callbacks?token={token}
	// as context.callback_urls.event. Any missing piece => omit callback_urls.
	CallbackSigner    CallbackSigner
	CallbackSecret    []byte
	CallbackPublicBase string
	CallbackTTL       time.Duration
}

// CallbackSigner issues a short-lived HMAC token bound to runID. It mirrors
// plugincallback.Issue so bootstrap can wire the real implementation without
// httpplugin depending on plugincallback directly at the type level.
type CallbackSigner func(secret []byte, runID string, ttl time.Duration) (token string, exp time.Time, err error)

// defaultCallbackTTL mirrors config.DefaultCallbackTokenTTLSec so callers that
// omit CallbackTTL still get a sane lifetime. Bootstrap passes the configured
// value explicitly.
const defaultCallbackTTL = time.Hour
