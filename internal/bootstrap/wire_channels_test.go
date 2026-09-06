package bootstrap

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

// stubBootChannel is a test-only channel that participates in full assembly:
// it registers via the public registry Descriptor API and implements
// channel.Bootstrapper. wireChannels must discover and wire it without any
// bootstrap-package changes.
type stubBootChannel struct {
	name string
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
	return rt, "", false, nil
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
