// Package webhook implements a generic out-of-process channel: IM adapters
// (independent processes, any language) exchange JSON over HTTP with baize.
// Each configured instance represents one IM account.
package webhook

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/channel"
)

func init() {
	channel.Register(channel.Descriptor{
		Name:             "webhook",
		Build:            func(c channel.Config) (channel.Channel, error) { return openFromConfig(c["name"], c) },
		DefaultCredsDir:  "",
		EnabledByDefault: false, // opt-in via config channels:; multiple instances allowed
		// webhook requires per-instance opaque config (secret/outbound_url/
		// assignee) and supports multiple named instances, so it is never
		// auto-wired once-per-type on the legacy (no channels:) path.
		DeclarativeOnly: true,
	})
}

// Channel is one webhook channel instance (one IM account).
type Channel struct {
	cfg    instanceConfig
	sender *sender
	rt     *channel.Runtime

	// accountMu guards activeAccount, the real IM account learned from the
	// first inbound message after adapter login (autostart adapters log in as
	// an account baize does not know statically). Outbound messages use it in
	// preference to the configured/static account.
	accountMu     sync.RWMutex
	activeAccount string

	// settingsDir holds persisted per-instance settings (settings.json);
	// empty means in-memory only (tests). settings/allowlist are the hot,
	// mutable state guarded by settingsMu. admin is the out-of-process
	// adapter management client (nil until task 10 wires the HTTP impl).
	settingsDir string
	settings    channel.ChannelSettings
	settingsMu  sync.RWMutex
	allowlist   map[string]bool
	// manualStop records that an operator stopped the adapter process via the
	// process-control plane, so Status() reports "stopped" rather than probing
	// the dead port and mislabeling it "start_failed". Cleared on start/restart
	// and by settings updates that re-enable the channel.
	manualStop bool
	admin      adminClient
	// sup hosts the out-of-process adapter for autostart instances; nil when
	// the adapter is deployed independently (admin_url only).
	sup *supervisor
	// lifeCtx/lifeCancel bound the supervisor watchdog to baize's lifetime;
	// cancelled in Stop() so a parked backoff loop exits without respawning.
	lifeCtx    context.Context
	lifeCancel context.CancelFunc
	// procMu serializes process-level control (start/stop/restart) driven by
	// the management plane so concurrent operators cannot spawn/kill races.
	procMu sync.Mutex
}

// bgCtx returns the context for adapter management calls made outside of an
// inbound request (settings reconcile, status probes).
func (c *Channel) bgCtx() context.Context { return context.Background() }

// setActiveAccount records the account learned from inbound (e.g. the post-
// login account reported by the adapter). Empty/whitespace values are ignored.
func (c *Channel) setActiveAccount(acct string) {
	acct = strings.TrimSpace(acct)
	if acct == "" {
		return
	}
	c.accountMu.Lock()
	c.activeAccount = acct
	c.accountMu.Unlock()
}

// activeAccountOr resolves the account to stamp on outbound messages: an
// explicit extras["account"] wins, then the learned activeAccount, then the
// statically configured account.
func (c *Channel) activeAccountOr(extras map[string]string) string {
	if extras != nil {
		if a := strings.TrimSpace(extras["account"]); a != "" {
			return a
		}
	}
	c.accountMu.RLock()
	a := c.activeAccount
	c.accountMu.RUnlock()
	if a != "" {
		return a
	}
	return c.cfg.Account
}

// openFromConfig builds a webhook instance. name is the instance name; it is
// required and used as the default source/account.
func openFromConfig(name string, m map[string]string) (*Channel, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("webhook: instance name is required (use channels[].name)")
	}
	if !safeInstanceName(name) {
		return nil, fmt.Errorf("webhook: instance name %q must be a single URL-safe segment (letters, digits, '-', '_', '.'); no '/' or spaces", name)
	}
	cfg, err := parseConfig(name, m)
	if err != nil {
		return nil, err
	}
	c := &Channel{cfg: cfg}
	// Sender holds a pointer to the live config so the secret finalized later
	// in Bootstrap (resolveSecret) is used when signing outbound.
	c.sender = newSender(&c.cfg)
	return c, nil
}

func (c *Channel) Name() string   { return c.cfg.Name }
func (c *Channel) Source() string { return c.cfg.Source }

// Start launches the autostart adapter child process (when configured) and
// reconciles the adapter's enabled state. Inbound still arrives over HTTP and
// outbound is request/response; there is no polling loop inside baize.
func (c *Channel) Start(ctx context.Context) error {
	if c.sup != nil {
		if err := c.sup.start(ctx); err != nil {
			return err
		}
	}
	c.reconcileEnabled(c.GetSettings().Enabled)
	return nil
}

// Stop asks the adapter to stop its polling and gracefully terminates any
// supervised child process (HMAC /admin/shutdown -> SIGTERM -> force kill).
// Errors are best-effort: shutdown proceeds regardless.
func (c *Channel) Stop(ctx context.Context) error {
	if c.admin != nil {
		_ = c.admin.Stop(ctx)
	}
	if c.sup != nil {
		if c.lifeCancel != nil {
			c.lifeCancel()
		}
		_ = c.sup.terminate(ctx)
	}
	return nil
}

// Bootstrap assembles the channel Runtime from generic deps. Per-instance
// settings (assignee/agent/allowlist/enabled) are loaded from
// <DataDir>/channels/webhook/<name>/settings.json when DataDir is set and
// overlaid hot on the config baseline; assignee/agent/vision otherwise come
// from instance config. It registers its inbound HTTP route via deps.Routes
// when available.
func (c *Channel) Bootstrap(deps channel.BuildDeps) (*channel.Runtime, string, bool, error) {
	rt := &channel.Runtime{
		Runs:           deps.Store,
		Meta:           deps.Meta,
		Messages:       deps.Messages,
		Assignee:       c.cfg.Assignee,
		DefaultAgentID: firstNonEmpty(c.cfg.AgentID, deps.DefaultAgentID),
		ResolveModel:   deps.ResolveModel,
		AfterCreateRun: deps.AfterCreateRun,
		ResumeHITL:     deps.ResumeHITL,
		Source:         c.cfg.Source,
		Media:          deps.Media,
	}
	c.rt = rt
	if dir := strings.TrimSpace(deps.DataDir); dir != "" {
		c.settingsDir = filepath.Join(dir, "channels", "webhook", c.cfg.Name)
	}
	if err := c.loadSettings(); err != nil {
		return nil, "", false, fmt.Errorf("webhook: load settings: %w", err)
	}
	// Make an auto-generated autostart secret stable across baize restarts so
	// a surviving (orphaned) adapter child keeps verifying baize's requests.
	// Must run before the admin client and supervisor are built (they use it).
	c.resolveSecret()
	// Management plane client (admin proxy + status). The secret converges for
	// autostart instances: OutboundSecret == the generated Secret == the
	// adapter's -secret.
	if c.cfg.AdminURL != "" {
		c.admin = newHTTPAdminClient(c.cfg.AdminURL, c.cfg.OutboundSecret)
	}
	// Autostart child process supervisor.
	if c.cfg.AdapterAutostart {
		// Resolve baize's loopback base for the adapter's -baize inbound URL:
		// deps.SelfBaseURL (when known) -> config adapter_baize_url -> default.
		baizeURL := strings.TrimSpace(deps.SelfBaseURL)
		if baizeURL == "" {
			baizeURL = strings.TrimSpace(c.cfg.AdapterBaizeURL)
		}
		if baizeURL == "" {
			baizeURL = "http://127.0.0.1:8080"
		}
		credsDir := c.cfg.AdapterCredsDir
		if credsDir == "" {
			credsDir = "./data/channels/" + c.cfg.Name
		}
		// The adapter POSTs inbound messages back to baize's inbound route;
		// its admin/outbound listeners live at AdminURL.
		inboundURL := strings.TrimRight(baizeURL, "/") + "/v0/channels/" + c.cfg.Name + "/inbound"
		args := append([]string(nil), c.cfg.AdapterArgs...)
		args = append(args,
			"-baize="+inboundURL,
			"-secret="+c.cfg.Secret,
			"-creds="+credsDir,
		)
		healthz := strings.TrimRight(c.cfg.AdminURL, "/") + "/healthz"
		sup := &supervisor{
			command:    c.cfg.AdapterCommand,
			args:       args,
			healthzURL: healthz,
		}
		// Signed compatibility check for the adopt-orphan path: a listener that
		// answers a signed /admin/status shares our secret; one that 401s is a
		// stale/foreign process holding the port.
		if c.admin != nil {
			sup.compatible = func(ctx context.Context) bool {
				probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				if _, _, err := c.admin.Status(probeCtx); err != nil {
					return false
				}
				return true
			}
		}
		// Bound the watchdog to baize's lifetime and wire the graceful
		// shutdown path: terminate() hits the HMAC /admin/shutdown endpoint
		// before escalating to SIGTERM/force kill.
		c.lifeCtx, c.lifeCancel = context.WithCancel(context.Background())
		sup.lifecycleCtx = c.lifeCtx
		if c.admin != nil {
			sup.gracefulShutdown = func(ctx context.Context) error {
				return c.admin.Shutdown(ctx)
			}
		}
		c.sup = sup
	}
	if deps.Routes != nil {
		deps.Routes.RegisterRoute(
			"POST /v0/channels/"+c.cfg.Name+"/inbound",
			c.inboundHandler(),
		)
	}
	// Independently deployed adapters (no supervised child) are reachable now,
	// so reconcile enabled state at assembly; autostart children reconcile in
	// Start() once the process is healthy.
	if c.sup == nil {
		c.reconcileEnabled(c.GetSettings().Enabled)
	}
	// Only autostart, enabled instances need baize to launch their loop.
	return rt, "", c.GetSettings().Enabled && c.cfg.AdapterAutostart, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// SendText pushes a text message to the adapter for peerID.
func (c *Channel) SendText(ctx context.Context, peerID, text string, extras map[string]string) error {
	acct := c.activeAccountOr(extras)
	msg := OutboundMessage{
		Kind:           kindFromExtras(extras),
		RunID:          runIDFromExtras(extras),
		ConversationID: channel.ConvID(c.cfg.Source, acct, peerID),
		Account:        acct,
		Peer:           Peer{ID: peerID},
		Text:           text,
		ContextToken:   extras["context_token"],
	}
	return c.sender.post(ctx, msg)
}

// SendMedia pushes a file to the adapter (small files inline base64).
func (c *Channel) SendMedia(ctx context.Context, peerID, filename, mime string, data []byte, extras map[string]string) error {
	acct := c.activeAccountOr(extras)
	msg := OutboundMessage{
		Kind:           kindFromExtras(extras),
		RunID:          runIDFromExtras(extras),
		ConversationID: channel.ConvID(c.cfg.Source, acct, peerID),
		Account:        acct,
		Peer:           Peer{ID: peerID},
		Media: []OutboundMedia{{
			Name:          filename,
			MIME:          mime,
			ContentBase64: base64.StdEncoding.EncodeToString(data),
		}},
		ContextToken: extras["context_token"],
	}
	return c.sender.post(ctx, msg)
}

func kindFromExtras(extras map[string]string) string {
	if extras != nil {
		if k := strings.TrimSpace(extras[channel.ExtraKind]); k != "" {
			return k
		}
	}
	return channel.OutboundKindAssistant
}

func runIDFromExtras(extras map[string]string) string {
	if extras != nil {
		return strings.TrimSpace(extras[channel.ExtraRunID])
	}
	return ""
}

// safeInstanceName reports whether name is a single URL path segment made only
// of unreserved, path-safe characters. It guards the self-registered inbound
// route "POST /v0/channels/<name>/inbound" against names that would add path
// segments ('/'), spaces, or otherwise break ServeMux pattern matching.
func safeInstanceName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			// ok
		case r == '-' || r == '_' || r == '.':
			// ok
		default:
			return false
		}
	}
	return true
}

var (
	_ channel.Channel        = (*Channel)(nil)
	_ channel.SourceSourced  = (*Channel)(nil)
	_ channel.Bootstrapper   = (*Channel)(nil)
	_ channel.ManagedChannel = (*Channel)(nil)
)
