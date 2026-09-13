package bootstrap

import (
	"path/filepath"
	"testing"

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
