package bootstrap

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
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
	// Faithful to the real channel contract (weixin.Bootstrap returns the
	// resolved creds dir it was built with), so handle.CredsDir reflects it.
	return rt, s.credsDir, false, nil
}

var _ channel.Bootstrapper = (*stubBootChannel)(nil)

// withEmptyRegistry replaces the global registry with an empty one for the
// test and restores the prior descriptors (production channels such as weixin
// are registered via init()) on cleanup.
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
	if _, ok := d.srv.Channel("weixin"); ok {
		t.Fatal("weixin handle must not be present when its descriptor is unregistered")
	}
}

// TestWireChannelsWiresAllRegisteredDescriptors verifies the production case:
// with the real registry (weixin registered via init), wireChannels assembles
// weixin, wires the router as the engine/api Outbound, and binds OutboundExtras.
func TestWireChannelsWiresAllRegisteredDescriptors(t *testing.T) {
	d := channelDepsForTest(t)
	router, err := wireChannels(d)
	if err != nil {
		t.Fatalf("wireChannels: %v", err)
	}
	if _, ok := router.For("weixin"); !ok {
		t.Fatal("router should route weixin by its Source()")
	}
	h, ok := d.srv.Channel("weixin")
	if !ok {
		t.Fatal("srv should have a weixin channel handle")
	}
	if h.Runtime == nil || h.Runtime.Source != "weixin" {
		t.Fatalf("weixin handle runtime not wired: %+v", h.Runtime)
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
// top of the production registry (weixin stays registered as the built-in
// default). The registry snapshot is restored on cleanup. It returns a pointer
// to the channel.Config captured at Build time.
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

// TestWireChannelsEmptyConfigWiresAll is the back-compat anchor: with no
// declarative channels section, every registered channel (the built-in
// weixin default + an extra optional stub) is wired, exactly as in task 5.
func TestWireChannelsEmptyConfigWiresAll(t *testing.T) {
	registerStubChannel(t, "stuball")

	d := channelDepsForTest(t) // zero config.Config => Channels == nil
	if _, err := wireChannels(d); err != nil {
		t.Fatalf("wireChannels: %v", err)
	}
	if _, ok := d.srv.Channel("weixin"); !ok {
		t.Fatal("empty config must wire built-in default weixin")
	}
	if _, ok := d.srv.Channel("stuball"); !ok {
		t.Fatal("empty config must wire every registered channel (stuball)")
	}
}

// TestWireChannelsDeclarativeFiltersOptional proves an optional channel is
// wired only when explicitly enabled, while the unlisted built-in default
// (weixin) stays wired for back-compat.
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
	if _, ok := d.srv.Channel("weixin"); !ok {
		t.Fatal("unlisted built-in default weixin must stay wired (back-compat)")
	}
}

// TestWireChannelsDeclarativeOmitsDisabledOptional proves an optional channel
// that is listed enabled:false is not wired, while the built-in default
// remains.
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
	if _, ok := d.srv.Channel("weixin"); !ok {
		t.Fatal("built-in default weixin must remain wired")
	}
}

// TestWireChannelsExplicitDisableWins proves enabled:false overrides the
// built-in default marker (explicit opt-out always wins).
func TestWireChannelsExplicitDisableWins(t *testing.T) {
	d := channelDepsForTest(t)
	d.cfg.Channels = []config.ChannelConfig{{Type: "weixin", Enabled: false}}
	if _, err := wireChannels(d); err != nil {
		t.Fatalf("wireChannels: %v", err)
	}
	if _, ok := d.srv.Channel("weixin"); ok {
		t.Fatal("weixin enabled:false must NOT be wired even though it is the default")
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

func channelDepsForTest(t *testing.T) channelDeps {
	t.Helper()
	// Hermetic cwd: weixin's DefaultCredsDir is relative ("./data/...");
	// chdir to a temp dir so a locally logged-in dev environment cannot make
	// the test start a real poll loop.
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
