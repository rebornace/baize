package api_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/channel/weixin"
	"github.com/rebornace/baize/internal/store"
)

func TestServerRegisterChannel(t *testing.T) {
	srv := api.NewServer(store.NewMemory(), nil, nil)

	// Unknown channel name is not found.
	if _, ok := srv.Channel("weixin"); ok {
		t.Fatal("weixin should not be registered before RegisterChannel")
	}

	dir := t.TempDir()
	fake := weixin.NewFake()
	rt := &channel.Runtime{Runs: store.NewMemory()}
	ch := weixin.New(fake, rt, "", "")
	ch.SetCredsDir(dir)
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.RegisterChannel(&api.ChannelHandle{
		Name:     "weixin",
		Channel:  ch,
		Runtime:  rt,
		CredsDir: dir,
		RunCtx:   runCtx,
	})

	h, ok := srv.Channel("weixin")
	if !ok {
		t.Fatal("weixin handle should be found after registration")
	}
	if h == nil {
		t.Fatal("returned handle is nil")
	}
	if h.Name != "weixin" {
		t.Fatalf("handle Name=%q want %q", h.Name, "weixin")
	}
	if h.CredsDir != dir {
		t.Fatalf("handle CredsDir=%q want %q", h.CredsDir, dir)
	}
	if h.Runtime != rt {
		t.Fatal("handle Runtime should be the injected runtime")
	}
	if h.Channel == nil {
		t.Fatal("handle Channel should not be nil")
	}
	gotCh, ok := h.Channel.(*weixin.Channel)
	if !ok || gotCh == nil {
		t.Fatal("handle Channel should be the registered *weixin.Channel")
	}
	if gotCh.Name() != weixin.SourceName {
		t.Fatalf("channel Name=%q want %q", gotCh.Name(), weixin.SourceName)
	}

	// Nil handle and empty-name handles are ignored (no panic, no entry).
	srv.RegisterChannel(nil)
	srv.RegisterChannel(&api.ChannelHandle{Name: "  ", Channel: ch})
	if _, ok := srv.Channel(""); ok {
		t.Fatal("empty-name handle should not be registered")
	}
	if _, ok := srv.Channel("nope"); ok {
		t.Fatal("unknown channel should not be found")
	}
}
