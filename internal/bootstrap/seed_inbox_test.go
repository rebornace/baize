package bootstrap

import (
	"encoding/json"
	"testing"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/settingscrypto"
	"github.com/rebornace/baize/internal/store"
)

func TestSeedInboxChannelsOpensSealedSecretForRegistry(t *testing.T) {
	const plainSecret = "inbox-secret-abcdefghij"
	const settingsKey = "test-settings-key-32bytes-ok!!"
	t.Setenv("BAIZE_SETTINGS_KEY", settingsKey)
	key, err := settingscrypto.KeyFromEnv()
	if err != nil || key == nil {
		t.Fatalf("KeyFromEnv: %v", err)
	}
	sealed, err := settingscrypto.Seal(key, plainSecret)
	if err != nil {
		t.Fatal(err)
	}

	st := store.NewMemory()
	channels := []inbox.Channel{{
		ID: "alerts", AgentID: "a", Secret: sealed, Enabled: true,
	}}
	raw, err := json.Marshal(channels)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(store.SettingKeyInboxChannels, raw); err != nil {
		t.Fatal(err)
	}

	reg := inbox.NewRegistry()
	if err := seedInboxChannels(config.Config{}, st, reg); err != nil {
		t.Fatal(err)
	}
	got, ok := reg.GetAny("alerts")
	if !ok {
		t.Fatal("channel not in registry")
	}
	if got.Secret != plainSecret {
		t.Fatalf("registry secret=%q want plaintext %q", got.Secret, plainSecret)
	}
	if settingscrypto.IsSealed(got.Secret) {
		t.Fatal("registry must hold plaintext secret for HMAC verification")
	}
}
