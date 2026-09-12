package bootstrap

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/channel/webhook"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/webhooksig"
)

// stubBootChannel is a test-only channel that participates in full assembly:
// it registers via the public registry Descriptor API and implements
// channel.Bootstrapper. wireChannels must discover and wire it without any
// bootstrap-package changes.
type stubBootChannel struct {
	name     string
	credsDir string
}

func (s *stubBootChannel) Name() string                { return s.name }
func (s *stubBootChannel) Start(context.Context) error { return nil }
func (s *stubBootChannel) Stop(context.Context) error  { return nil }
func (s *stubBootChannel) SendText(context.Context, string, string, map[string]string) error {
	return nil
}
func (s *stubBootChannel) SendMedia(context.Context, string, string, string, []byte, map[string]string) error {
	return nil
}
func (s *stubBootChannel) Source() string { return s.name }

func (s *stubBootChannel) Bootstrap(deps channel.BuildDeps) (*channel.Runtime, string, bool, error) {
	rt := &channel.Runtime{
		Runs:           deps.Store,
		Meta:           deps.Meta,
		Messages:       deps.Messages,
		Assignee:       "a",
		DefaultAgentID: "b",
		Source:         s.name,
	}
	// Faithful to the real channel contract (a channel's Bootstrap returns
	// the resolved creds dir it was built with), so handle.CredsDir reflects
	// it.
	return rt, s.credsDir, false, nil
}

var _ channel.Bootstrapper = (*stubBootChannel)(nil)

// withEmptyRegistry replaces the global registry with an empty one for the
// test and restores the prior descriptors (production channels such as the
// webhook channel are registered via init()) on cleanup.
func withEmptyRegistry(t *testing.T) {
	t.Helper()
	before := channel.Descriptors()
	channel.ResetForTest()
	t.Cleanup(func() {
		channel.ResetForTest()
		for _, d := range before {
			channel.Register(d)
		}
	})
}

// TestWireChannelsPicksUpRegisteredDescriptor proves the generic assembly
// loop discovers a channel registered only through the registry: it is
// instantiated, bootstrapped, added to the router under its Source(), and
// registered as an api channel handle.
func TestWireChannelsPicksUpRegisteredDescriptor(t *testing.T) {
	withEmptyRegistry(t)
	channel.Register(channel.Descriptor{
		Name:  "stubch",
		Build: func(channel.Config) (channel.Channel, error) { return &stubBootChannel{name: "stubch"}, nil },
	})

	d := channelDepsForTest(t)
	router, err := wireChannels(d)
	if err != nil {
		t.Fatalf("wireChannels: %v", err)
	}
	if _, ok := router.For("stubch"); !ok {
		t.Fatal("router should route stubch by its Source()")
	}
	h, ok := d.srv.Channel("stubch")
	if !ok {
		t.Fatal("srv should have a stubch channel handle")
	}
	if h.Runtime == nil || h.Runtime.Source != "stubch" {
		t.Fatalf("stubch handle runtime not wired: %+v", h.Runtime)
	}
	if _, ok := d.srv.Channel("webhook"); ok {
		t.Fatal("webhook handle must not be present on the legacy path (DeclarativeOnly)")
	}
}

// registerDefaultStub registers a Bootstrapper test channel with
// EnabledByDefault=true (mimicking a built-in default) on top of the
// production registry. The registry snapshot is restored on cleanup.
func registerDefaultStub(t *testing.T, name string) {
	t.Helper()
	before := channel.Descriptors()
	channel.Register(channel.Descriptor{
		Name:             name,
		Build:            func(channel.Config) (channel.Channel, error) { return &stubBootChannel{name: name}, nil },
		DefaultCredsDir:  "./data/channels/" + name,
		EnabledByDefault: true,
	})
	t.Cleanup(func() {
		channel.ResetForTest()
		for _, d := range before {
			channel.Register(d)
		}
	})
}

// TestWireChannelsWiresAllRegisteredDescriptors verifies the production
// wiring: with a built-in default descriptor present (EnabledByDefault), the
// legacy path assembles it, wires the router as the engine/api Outbound, and
// binds OutboundExtras. The production webhook descriptor is skipped on the
// legacy path (DeclarativeOnly).
func TestWireChannelsWiresAllRegisteredDescriptors(t *testing.T) {
	registerDefaultStub(t, "builtin")

	d := channelDepsForTest(t)
	router, err := wireChannels(d)
	if err != nil {
		t.Fatalf("wireChannels: %v", err)
	}
	if _, ok := router.For("builtin"); !ok {
		t.Fatal("router should route builtin by its Source()")
	}
	h, ok := d.srv.Channel("builtin")
	if !ok {
		t.Fatal("srv should have a builtin channel handle")
	}
	if h.Runtime == nil || h.Runtime.Source != "builtin" {
		t.Fatalf("builtin handle runtime not wired: %+v", h.Runtime)
	}
	if _, ok := d.srv.Channel("webhook"); ok {
		t.Fatal("webhook must not be auto-wired on the legacy path (DeclarativeOnly)")
	}
	if d.srv.Outbound != channel.Channel(router) {
		t.Fatal("srv.Outbound should be the router")
	}
	if d.engine.Outbound != channel.Channel(router) {
		t.Fatal("engine.Outbound should be the router")
	}
	if d.srv.OutboundExtras == nil || d.engine.OutboundExtras == nil {
		t.Fatal("OutboundExtras should be wired to router.Extras")
	}
	if d.engine.Meta == nil {
		t.Fatal("engine.Meta should be wired to the conversation meta store")
	}
}

// registerStubChannel registers an optional Bootstrapper test channel under
// name (EnabledByDefault left false, so it behaves like an add-on channel) on
// top of the production registry. The registry snapshot is restored on
// cleanup. It returns a pointer to the channel.Config captured at Build time.
func registerStubChannel(t *testing.T, name string) *channel.Config {
	t.Helper()
	before := channel.Descriptors()
	got := channel.Config{}
	channel.Register(channel.Descriptor{
		Name: name,
		Build: func(c channel.Config) (channel.Channel, error) {
			for k, v := range c {
				got[k] = v
			}
			return &stubBootChannel{name: name, credsDir: c["creds_dir"]}, nil
		},
		DefaultCredsDir: "./data/channels/" + name,
	})
	t.Cleanup(func() {
		channel.ResetForTest()
		for _, d := range before {
			channel.Register(d)
		}
	})
	return &got
}

// TestWireChannelsEmptyConfigWiresAll is the legacy-path anchor: with no
// declarative channels section, every non-DeclarativeOnly descriptor (an extra
// optional stub) is wired, while DeclarativeOnly types (the webhook channel)
// are skipped. With the in-process weixin channel gone, the legacy path wires
// no IM channel unless test descriptors are registered.
func TestWireChannelsEmptyConfigWiresAll(t *testing.T) {
	registerStubChannel(t, "stuball")

	d := channelDepsForTest(t) // zero config.Config => Channels == nil
	if _, err := wireChannels(d); err != nil {
		t.Fatalf("wireChannels: %v", err)
	}
	if _, ok := d.srv.Channel("webhook"); ok {
		t.Fatal("webhook (DeclarativeOnly) must not be auto-wired on the legacy path")
	}
	if _, ok := d.srv.Channel("stuball"); !ok {
		t.Fatal("empty config must wire every non-DeclarativeOnly channel (stuball)")
	}
}

// TestWireChannelsDeclarativeFiltersOptional proves an optional channel is
// wired only when explicitly enabled, while DeclarativeOnly types (the webhook
// channel) stay unwired unless explicitly listed.
func TestWireChannelsDeclarativeFiltersOptional(t *testing.T) {
	registerStubChannel(t, "stubon")

	d := channelDepsForTest(t)
	d.cfg.Channels = []config.ChannelConfig{{Type: "stubon", Enabled: true}}
	if _, err := wireChannels(d); err != nil {
		t.Fatalf("wireChannels: %v", err)
	}
	if _, ok := d.srv.Channel("stubon"); !ok {
		t.Fatal("stubon explicitly enabled must be wired")
	}
	if _, ok := d.srv.Channel("webhook"); ok {
		t.Fatal("webhook must not be wired unless explicitly listed as an instance")
	}
}

// TestWireChannelsDeclarativeOmitsDisabledOptional proves an optional channel
// that is listed enabled:false is not wired.
func TestWireChannelsDeclarativeOmitsDisabledOptional(t *testing.T) {
	registerStubChannel(t, "stuboff")

	d := channelDepsForTest(t)
	d.cfg.Channels = []config.ChannelConfig{{Type: "stuboff", Enabled: false}}
	if _, err := wireChannels(d); err != nil {
		t.Fatalf("wireChannels: %v", err)
	}
	if _, ok := d.srv.Channel("stuboff"); ok {
		t.Fatal("stuboff enabled:false must NOT be wired")
	}
	if _, ok := d.srv.Channel("webhook"); ok {
		t.Fatal("webhook must not be wired unless explicitly listed as an instance")
	}
}

// TestWireChannelsExplicitDisableWins proves enabled:false overrides the
// built-in default marker (explicit opt-out always wins).
func TestWireChannelsExplicitDisableWins(t *testing.T) {
	registerDefaultStub(t, "builtin")

	d := channelDepsForTest(t)
	d.cfg.Channels = []config.ChannelConfig{{Type: "builtin", Enabled: false}}
	if _, err := wireChannels(d); err != nil {
		t.Fatalf("wireChannels: %v", err)
	}
	if _, ok := d.srv.Channel("builtin"); ok {
		t.Fatal("builtin enabled:false must NOT be wired even though it is the default")
	}
}

// TestWireChannelsCredsDirOverride proves the declarative config.creds_dir
// overrides the descriptor DefaultCredsDir and reaches both the channel Build
// and the api handle; other opaque keys pass through.
func TestWireChannelsCredsDirOverride(t *testing.T) {
	got := registerStubChannel(t, "stubcreds")

	d := channelDepsForTest(t)
	d.cfg.Channels = []config.ChannelConfig{{
		Type:    "stubcreds",
		Enabled: true,
		Config:  map[string]string{"creds_dir": "./custom/stub", "base_url": "http://example"},
	}}
	if _, err := wireChannels(d); err != nil {
		t.Fatalf("wireChannels: %v", err)
	}
	if (*got)["creds_dir"] != "./custom/stub" {
		t.Fatalf("Build creds_dir=%q want ./custom/stub", (*got)["creds_dir"])
	}
	if (*got)["base_url"] != "http://example" {
		t.Fatalf("Build base_url=%q want http://example", (*got)["base_url"])
	}
	h, ok := d.srv.Channel("stubcreds")
	if !ok {
		t.Fatal("stubcreds handle missing")
	}
	if h.CredsDir != "./custom/stub" {
		t.Fatalf("handle CredsDir=%q want ./custom/stub", h.CredsDir)
	}
}

// signedInboundRequest builds a POST request to path carrying a valid channel
// HMAC signature over body, so the webhook inbound handler runs past its own
// signature check (the control-plane gate leaves channel inbound routes at
// RoleNone). A body without peer.id then reaches handler logic and yields 400.
func signedInboundRequest(t *testing.T, path, secret string, body []byte) *http.Request {
	t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := webhooksig.Sign(secret, ts, body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set(webhook.HeaderTimestamp, ts)
	req.Header.Set(webhook.HeaderSignature, sig)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestWireChannelsWebhookMultiInstance proves a webhook type configured with
// two named instances yields two separately-registered handles, two routable
// sources, and two distinct inbound routes mounted through the gated handler.
func TestWireChannelsWebhookMultiInstance(t *testing.T) {
	d := channelDepsForTest(t)
	d.cfg.Channels = []config.ChannelConfig{
		{Name: "feishu", Type: "webhook", Enabled: true, Config: map[string]string{
			"source": "feishu", "account": "feishu-bot", "secret": "s",
			"outbound_url": "http://x/o", "assignee": "u",
		}},
		{Name: "dingtalk", Type: "webhook", Enabled: true, Config: map[string]string{
			"source": "dingtalk", "account": "dt-bot", "secret": "s2",
			"outbound_url": "http://y/o", "assignee": "u",
		}},
	}
	router, err := wireChannels(d)
	if err != nil {
		t.Fatalf("wireChannels: %v", err)
	}
	if _, ok := router.For("feishu"); !ok {
		t.Fatal("router should route feishu source")
	}
	if _, ok := router.For("dingtalk"); !ok {
		t.Fatal("router should route dingtalk source")
	}
	if _, ok := d.srv.Channel("feishu"); !ok {
		t.Fatal("handle feishu missing")
	}
	if _, ok := d.srv.Channel("dingtalk"); !ok {
		t.Fatal("handle dingtalk missing")
	}
	// Inbound routes must be mounted (not 404) and public to the channel HMAC
	// path (not 401/403 from the control-plane gate). A correctly signed body
	// with no peer.id reaches the handler and returns 400.
	for _, c := range []struct{ name, secret string }{
		{"feishu", "s"}, {"dingtalk", "s2"},
	} {
		p := "/v0/channels/" + c.name + "/inbound"
		req := signedInboundRequest(t, p, c.secret, []byte(`{"event":"message"}`))
		rec := httptest.NewRecorder()
		d.srv.Handler().ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("route %s not mounted", p)
		}
		if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
			t.Fatalf("inbound route %s should be public (channel HMAC), got %d", p, rec.Code)
		}
	}
}

// TestWireChannelsWebhookDuplicateInstanceNameErrors proves two instances with
// the same name fail fast.
func TestWireChannelsWebhookDuplicateInstanceNameErrors(t *testing.T) {
	d := channelDepsForTest(t)
	d.cfg.Channels = []config.ChannelConfig{
		{Name: "dup", Type: "webhook", Enabled: true, Config: map[string]string{"secret": "s", "outbound_url": "http://x/o", "assignee": "u"}},
		{Name: "dup", Type: "webhook", Enabled: true, Config: map[string]string{"secret": "s", "outbound_url": "http://y/o", "assignee": "u"}},
	}
	if _, err := wireChannels(d); err == nil {
		t.Fatal("expected error for duplicate instance name")
	}
}

// TestWireChannelsWebhookDuplicateSourceErrors proves two instances with
// distinct names but the same config.source fail fast instead of silently
// overwriting each other in the source-keyed router.
func TestWireChannelsWebhookDuplicateSourceErrors(t *testing.T) {
	d := channelDepsForTest(t)
	d.cfg.Channels = []config.ChannelConfig{
		{Name: "alpha", Type: "webhook", Enabled: true, Config: map[string]string{
			"source": "shared", "secret": "s", "outbound_url": "http://x/o", "assignee": "u",
		}},
		{Name: "beta", Type: "webhook", Enabled: true, Config: map[string]string{
			"source": "shared", "secret": "s2", "outbound_url": "http://y/o", "assignee": "u",
		}},
	}
	_, err := wireChannels(d)
	if err == nil {
		t.Fatal("expected error for duplicate channel source")
	}
	if !strings.Contains(err.Error(), "duplicate channel source") || !strings.Contains(err.Error(), "shared") {
		t.Fatalf("error should mention duplicate source %q, got: %v", "shared", err)
	}
}

// TestWireChannelsWebhookNotWiredInEmptyConfig is the legacy-path anchor: the
// webhook channel registers via the blank import in bootstrap.go, but with no
// channels: section it must NOT be auto-wired (DeclarativeOnly). With the
// in-process weixin channel removed, the legacy path wires no IM channel at
// all; IM accounts are declared explicitly via channels: webhook instances.
func TestWireChannelsWebhookNotWiredInEmptyConfig(t *testing.T) {
	d := channelDepsForTest(t) // zero config.Config => Channels == nil
	if _, err := wireChannels(d); err != nil {
		t.Fatalf("wireChannels with empty config must not error: %v", err)
	}
	if _, ok := d.srv.Channel("webhook"); ok {
		t.Fatal("webhook must not be auto-wired in the legacy/empty-config path")
	}
	if _, ok := d.srv.Channel("weixin"); ok {
		t.Fatal("no in-process weixin channel exists; it must never be wired")
	}
}

func channelDepsForTest(t *testing.T) channelDeps {
	t.Helper()
	// Hermetic cwd: channel creds dirs are relative ("./data/..."); chdir to
	// a temp dir so a locally logged-in dev environment cannot make the test
	// touch real channel state.
	t.Chdir(t.TempDir())
	st := store.NewMemory()
	messages := conversation.NewMemoryStore()
	engine := &run.Engine{Store: st, Messages: messages}
	srv := api.NewServer(st, nil, engine)
	srv.DefaultAgentID = "agent-default"
	closer := &storeAndMCPCloser{}
	t.Cleanup(func() { _ = closer.Close() })
	return channelDeps{
		srv:            srv,
		st:             st,
		messages:       messages,
		engine:         engine,
		provider:       nil,
		defaultAgentID: "agent-default",
		runCtx:         t.Context(),
		closer:         closer,
	}
}
