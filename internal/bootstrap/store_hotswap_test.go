package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/store"
)

func TestHotSwapMemoryToSQLite(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
	dir := t.TempDir()
	cfg := config.Config{}
	cfg.Store.Driver = "memory"
	cfg.Agent.ID = "agent-1"
	cfg.Agent.System = "sys"
	cfg.LLM.Provider = "mock"
	cfg.Listen = "127.0.0.1:0"
	cfg.Run.MaxSteps = 4
	cfg.Conversation.MaxMessages = 10

	srv, closer, err := newAPIServer(cfg, "")
	if err != nil {
		t.Fatalf("newAPIServer: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })

	if srv.HotSwapStore == nil {
		t.Fatal("HotSwapStore not wired")
	}

	old := srv.Store
	run, err := old.CreateRun(store.CreateRunInput{AgentID: "agent-1", Input: "hi"})
	if err != nil {
		t.Fatalf("create on memory: %v", err)
	}
	oldID := run.ID

	dbPath := filepath.Join(dir, "hot.db")
	if err := srv.HotSwapStore(config.StoreOverlay{
		Driver:     "sqlite",
		SQLitePath: dbPath,
	}); err != nil {
		t.Fatalf("HotSwap: %v", err)
	}

	if srv.Store == old {
		t.Fatal("Store pointer not swapped")
	}
	if _, err := srv.Store.GetRun(oldID); err == nil {
		t.Fatal("old run should not exist on new sqlite store")
	}
	run2, err := srv.Store.CreateRun(store.CreateRunInput{AgentID: "agent-1", Input: "new"})
	if err != nil {
		t.Fatalf("create on sqlite: %v", err)
	}
	got, err := srv.Store.GetRun(run2.ID)
	if err != nil || got.Input != "new" {
		t.Fatalf("read back: %#v err=%v", got, err)
	}
	if srv.EffectiveStoreDriver != nil && srv.EffectiveStoreDriver() != "sqlite" {
		t.Fatalf("effective driver=%q", srv.EffectiveStoreDriver())
	}
}

func TestHotSwapOpenFailureKeepsStore(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
	cfg := config.Config{}
	cfg.Store.Driver = "memory"
	cfg.Agent.ID = "agent-1"
	cfg.LLM.Provider = "mock"
	cfg.Run.MaxSteps = 4

	srv, closer, err := newAPIServer(cfg, "")
	if err != nil {
		t.Fatalf("newAPIServer: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })

	old := srv.Store
	err = srv.HotSwapStore(config.StoreOverlay{
		Driver: "postgres",
		DSN:    "host=127.0.0.1 port=1 user=no dbname=no sslmode=disable connect_timeout=1",
	})
	if err == nil {
		t.Fatal("expected open failure")
	}
	if srv.Store != old {
		t.Fatal("Store must not change on failed HotSwap")
	}
}

// TestHotSwapUpdatesWebhookChannelOutboxStore locks the bootstrap hot-swap
// path: ForEachChannel + SetStore runs with Runtime.Runs, so post-swap
// SendText writes channel_outbox on the new store (worker still drains).
func TestHotSwapUpdatesWebhookChannelOutboxStore(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
	dir := t.TempDir()
	t.Chdir(dir)

	outbound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(outbound.Close)

	cfg := config.Config{}
	cfg.Store.Driver = "memory"
	cfg.Agent.ID = "agent-1"
	cfg.Agent.System = "sys"
	cfg.LLM.Provider = "mock"
	cfg.Listen = "127.0.0.1:0"
	cfg.Run.MaxSteps = 4
	cfg.Conversation.MaxMessages = 10
	cfg.Channels = []config.ChannelConfig{{
		Name: "weixin", Type: "webhook", Enabled: true,
		Config: map[string]string{
			"source": "weixin", "account": "acc", "secret": "s",
			"outbound_url": outbound.URL, "assignee": "u",
		},
	}}

	srv, closer, err := newAPIServer(cfg, "")
	if err != nil {
		t.Fatalf("newAPIServer: %v", err)
	}
	t.Cleanup(func() { _ = closer.Close() })

	h, ok := srv.Channel("weixin")
	if !ok || h.Channel == nil || h.Runtime == nil {
		t.Fatal("weixin handle / runtime missing after wire")
	}
	oldStore := srv.Store
	if h.Runtime.Runs != oldStore {
		t.Fatal("Runtime.Runs should match pre-swap store")
	}

	dbPath := filepath.Join(dir, "hot-outbox.db")
	if err := srv.HotSwapStore(config.StoreOverlay{
		Driver:     "sqlite",
		SQLitePath: dbPath,
	}); err != nil {
		t.Fatalf("HotSwap: %v", err)
	}
	if srv.Store == oldStore {
		t.Fatal("Store pointer not swapped")
	}
	if h.Runtime.Runs != srv.Store {
		t.Fatal("Runtime.Runs not updated with swapped store")
	}

	if err := h.Channel.SendText(context.Background(), "peer1", "after-swap", map[string]string{
		channel.ExtraKind: channel.OutboundKindAssistant,
		"run_id":          "run_hotswap",
	}); err != nil {
		t.Fatalf("SendText after HotSwap: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		list, err := srv.Store.ListChannelOutbox("weixin", []store.ChannelOutboxStatus{
			store.ChannelOutboxPending, store.ChannelOutboxDelivered,
		}, 5)
		if err != nil {
			t.Fatalf("ListChannelOutbox on new store: %v", err)
		}
		if len(list) > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("expected channel_outbox row on swapped store after SendText")
}
