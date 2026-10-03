package bootstrap

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/settingscrypto"
	"github.com/rebornace/baize/internal/store"
)

func TestNewAPIServerRejectsSealedStoreWithoutSettingsKey(t *testing.T) {
	const key = "test-settings-key-32bytes-ok!!"
	t.Setenv("BAIZE_SETTINGS_KEY", key)
	dbPath := filepath.Join(t.TempDir(), "migrate.db")
	cfg := minimalControlPlaneCfg(t)
	cfg.Store.Driver = "sqlite"
	cfg.Store.SQLitePath = dbPath

	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertModelProfile(store.ModelProfile{
		Name: "n", Provider: "openai_compatible", Model: "m", APIKey: "sk-sealed-boot",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BAIZE_SETTINGS_KEY", "")
	_, _, err = newAPIServer(cfg, "")
	if err == nil {
		t.Fatal("expected startup failure for sealed secrets without key")
	}
	if !errors.Is(err, settingscrypto.ErrCiphertext) {
		t.Fatalf("want ErrCiphertext, got %v", err)
	}
}
