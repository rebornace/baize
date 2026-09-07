package channel

import (
	"context"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

// BuildDeps are the generic dependencies every wired channel shares. A
// Bootstrapper channel builds its own Runtime (assignee/agent/source come
// from its persisted per-channel settings) from these deps.
type BuildDeps struct {
	Store          RunStore
	Meta           conversation.MetaStore
	Messages       conversation.Store
	DefaultAgentID string
	// SupportsVision controls whether inbound image attachments become
	// multimodal LLM parts.
	SupportsVision bool
	// AfterCreateRun enqueues the run after inbound CreateRun (engine wiring).
	AfterCreateRun func(ctx context.Context, run *store.Run, userParts []llm.ContentPart) error
	// ResumeHITL continues a waiting_human run after an approve/reject reply.
	ResumeHITL func(ctx context.Context, runID string, approve bool, comment string) error
	// Routes, when non-nil, lets a channel mount its own HTTP endpoints during
	// Bootstrap (e.g. inbound webhook). Nil in tests that do not exercise HTTP.
	Routes RouteRegistrar
	// DataDir is the baize data directory for persisted per-channel state
	// (e.g. <dataDir>/channels/webhook/<name>/settings.json). Empty means
	// in-memory only (tests).
	DataDir string
}

// Bootstrapper is implemented by channels that participate in full assembly:
// they build their own Runtime from persisted per-channel settings, apply
// channel-specific state (allowlist, credentials), and report whether their
// inbound loop should start. Channels not implementing it are registered for
// future use but not started/wired.
type Bootstrapper interface {
	// Bootstrap assembles the channel Runtime. credsDir is the resolved
	// on-disk settings/creds directory; start reports whether the channel's
	// poll loop should be started (creds present && settings enabled).
	Bootstrap(deps BuildDeps) (rt *Runtime, credsDir string, start bool, err error)
}
