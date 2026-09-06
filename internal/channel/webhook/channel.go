// Package webhook implements a generic out-of-process channel: IM adapters
// (independent processes, any language) exchange JSON over HTTP with baize.
// Each configured instance represents one IM account.
package webhook

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

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
	return &Channel{cfg: cfg, sender: newSender(cfg)}, nil
}

func (c *Channel) Name() string   { return c.cfg.Name }
func (c *Channel) Source() string { return c.cfg.Source }

// Start/Stop are no-ops: the webhook channel has no polling loop; inbound
// arrives over HTTP and outbound is request/response.
func (c *Channel) Start(ctx context.Context) error { return nil }
func (c *Channel) Stop(ctx context.Context) error  { return nil }

// Bootstrap assembles the channel Runtime from generic deps. Persisted
// per-channel settings are not used for webhook (it is purely config-driven);
// assignee/agent/vision come from instance config. It registers its inbound
// HTTP route via deps.Routes when available.
func (c *Channel) Bootstrap(deps channel.BuildDeps) (*channel.Runtime, string, bool, error) {
	rt := &channel.Runtime{
		Runs:           deps.Store,
		Meta:           deps.Meta,
		Messages:       deps.Messages,
		Assignee:       c.cfg.Assignee,
		DefaultAgentID: firstNonEmpty(c.cfg.AgentID, deps.DefaultAgentID),
		SupportsVision: deps.SupportsVision || c.cfg.SupportsVision,
		AfterCreateRun: deps.AfterCreateRun,
		ResumeHITL:     deps.ResumeHITL,
		Source:         c.cfg.Source,
	}
	c.rt = rt
	if deps.Routes != nil {
		deps.Routes.RegisterRoute(
			"POST /v0/channels/"+c.cfg.Name+"/inbound",
			c.inboundHandler(),
		)
	}
	// start=true keeps parity with other Bootstrappers; Start is a no-op.
	return rt, "", true, nil
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
	msg := OutboundMessage{
		Kind:           kindFromExtras(extras),
		RunID:          runIDFromExtras(extras),
		ConversationID: channel.ConvID(c.cfg.Source, c.cfg.Account, peerID),
		Account:        c.cfg.Account,
		Peer:           Peer{ID: peerID},
		Text:           text,
		ContextToken:   extras["context_token"],
	}
	return c.sender.post(ctx, msg)
}

// SendMedia pushes a file to the adapter (small files inline base64).
func (c *Channel) SendMedia(ctx context.Context, peerID, filename, mime string, data []byte, extras map[string]string) error {
	msg := OutboundMessage{
		Kind:           kindFromExtras(extras),
		RunID:          runIDFromExtras(extras),
		ConversationID: channel.ConvID(c.cfg.Source, c.cfg.Account, peerID),
		Account:        c.cfg.Account,
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
	_ channel.Channel       = (*Channel)(nil)
	_ channel.SourceSourced = (*Channel)(nil)
	_ channel.Bootstrapper  = (*Channel)(nil)
)
