package channel

import (
	"context"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

// MediaStore persists inbound channel attachments (images and files) to a
// blob-backed namespace and gives back browser-reachable relative URLs, so
// media from IM channels (e.g. WeChat) renders inline (images) or downloads
// (files) in the web UI instead of showing only a bare "（附件：…）" note. It
// is optional: when nil, channel attachments are only sent to the model (text
// extracted, images when vision-capable) and named in text.
type MediaStore interface {
	// SaveInboundImage stores an inbound image for convID and returns its
	// relative GET URL (served with the same conversation ACL) along with the
	// sanitized blob object name.
	SaveInboundImage(ctx context.Context, convID, filename, mime string, data []byte) (url string, object string, err error)
	// SaveInboundFile stores a non-image inbound attachment (docx/pdf/zip/…)
	// and returns its relative download URL. It preserves the original file
	// extension so the download keeps a usable type.
	SaveInboundFile(ctx context.Context, convID, filename, mime string, data []byte) (url string, object string, err error)
}

// BuildDeps are the generic dependencies every wired channel shares. A
// Bootstrapper channel builds its own Runtime (assignee/agent/source come
// from its persisted per-channel settings) from these deps.
type BuildDeps struct {
	Store          RunStore
	Meta           conversation.MetaStore
	Messages       conversation.Store
	DefaultAgentID string
	// Media optionally persists inbound channel images for inline web display.
	Media MediaStore
	// SupportsVision controls whether inbound image attachments become
	// multimodal LLM parts.
	SupportsVision bool
	// VisionModelProfileID optionally resolves the id of a vision-capable
	// model profile used for inbound image messages when the default model is
	// text-only. Read per inbound message (hot reload: adding a vision model
	// takes effect without restart). Returning "" degrades images to text.
	VisionModelProfileID func() string
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
	// SelfBaseURL is baize's own loopback base URL, passed to autostart
	// adapter children so they POST inbound messages back to baize. Empty in
	// tests / when the listen port is not known at assembly time; the webhook
	// supervisor then falls back to config adapter_baize_url, then the
	// loopback default http://127.0.0.1:8080.
	SelfBaseURL string
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
