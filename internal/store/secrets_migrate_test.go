package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/settingscrypto"
)

func TestMigrateStoreEncryptsPlainAPIKey(t *testing.T) {
	setTestSettingsKey(t)
	st := NewMemory()
	st.PutModelProfilePlainForTest(ModelProfile{
		ID:       "mp1",
		Name:     "m",
		Provider: "openai_compatible",
		Model:    "gpt",
		APIKey:   "sk-plain-migrate",
	})

	if err := MigrateStore(st); err != nil {
		t.Fatal(err)
	}
	raw := st.modelProfiles["mp1"].APIKey
	if !strings.HasPrefix(raw, settingscrypto.Prefix) {
		t.Fatalf("stored api_key should be sealed, got %q", raw)
	}
	got, err := st.GetModelProfile("mp1")
	if err != nil || got.APIKey != "sk-plain-migrate" {
		t.Fatalf("read back plain key: %q err=%v", got.APIKey, err)
	}
	sealedAfterFirst := st.modelProfiles["mp1"].APIKey
	if err := MigrateStore(st); err != nil {
		t.Fatal(err)
	}
	if st.modelProfiles["mp1"].APIKey != sealedAfterFirst {
		t.Fatalf("second migrate rewrote sealed api_key: was %q now %q", sealedAfterFirst, st.modelProfiles["mp1"].APIKey)
	}
}

func TestMigrateStoreSkipsAlreadySealedProfileRaw(t *testing.T) {
	setTestSettingsKey(t)
	key, _ := settingscrypto.KeyFromEnv()
	sealed, err := settingscrypto.Seal(key, "sk-already")
	if err != nil {
		t.Fatal(err)
	}
	st := NewMemory()
	st.PutModelProfilePlainForTest(ModelProfile{
		ID: "mp1", Name: "m", Provider: "openai_compatible", Model: "gpt", APIKey: sealed,
	})
	if err := MigrateStore(st); err != nil {
		t.Fatal(err)
	}
	if st.modelProfiles["mp1"].APIKey != sealed {
		t.Fatalf("migrate must not re-seal stored ciphertext")
	}
}

func TestMigrateStoreSealsRuntimeCredsPlaintext(t *testing.T) {
	setTestSettingsKey(t)
	st := NewMemory()
	raw, _ := json.Marshal(map[string]interface{}{
		"creds": map[string]string{
			"admin_token": "adm-plain",
		},
	})
	if err := st.UpsertSetting(SettingKeyRuntimeSettings, raw); err != nil {
		t.Fatal(err)
	}
	if err := MigrateStore(st); err != nil {
		t.Fatal(err)
	}
	got, ok, err := st.GetSetting(SettingKeyRuntimeSettings)
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(string(got), "bz1:") {
		t.Fatalf("expected sealed admin_token, got %s", got)
	}
	afterFirst, _, _ := st.GetSetting(SettingKeyRuntimeSettings)
	if err := MigrateStore(st); err != nil {
		t.Fatal(err)
	}
	afterSecond, _, _ := st.GetSetting(SettingKeyRuntimeSettings)
	if string(afterFirst) != string(afterSecond) {
		t.Fatalf("second migrate changed runtime_settings bytes")
	}
}

func TestMigrateStoreSealsInboxSecretPlaintext(t *testing.T) {
	setTestSettingsKey(t)
	st := NewMemory()
	ch := []inbox.Channel{{ID: "c1", AgentID: "a", Secret: "inbox-secret", Enabled: true}}
	raw, _ := json.Marshal(ch)
	if err := st.UpsertSetting(SettingKeyInboxChannels, raw); err != nil {
		t.Fatal(err)
	}
	if err := MigrateStore(st); err != nil {
		t.Fatal(err)
	}
	got, _, _ := st.GetSetting(SettingKeyInboxChannels)
	if !strings.Contains(string(got), "bz1:") {
		t.Fatalf("expected sealed secret, got %s", got)
	}
	afterFirst := append([]byte(nil), got...)
	if err := MigrateStore(st); err != nil {
		t.Fatal(err)
	}
	got2, _, _ := st.GetSetting(SettingKeyInboxChannels)
	if string(afterFirst) != string(got2) {
		t.Fatalf("second migrate changed inbox_channels bytes")
	}
}

func TestMigrateStoreRejectsSealedWithoutKey(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", testSettingsKey)
	key, _ := settingscrypto.KeyFromEnv()
	sealed, err := settingscrypto.Seal(key, "adm1")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BAIZE_SETTINGS_KEY", "")

	st := NewMemory()
	raw, _ := json.Marshal(map[string]interface{}{
		"creds": map[string]string{"admin_token": sealed},
	})
	if err := st.UpsertSetting(SettingKeyRuntimeSettings, raw); err != nil {
		t.Fatal(err)
	}
	err = MigrateStore(st)
	if !errors.Is(err, settingscrypto.ErrCiphertext) {
		t.Fatalf("want ErrCiphertext, got %v", err)
	}
}

func TestMigrateStoreNoKeyPlaintextNoOp(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "")
	st := NewMemory()
	raw, _ := json.Marshal(map[string]interface{}{
		"creds": map[string]string{"admin_token": "plain"},
	})
	if err := st.UpsertSetting(SettingKeyRuntimeSettings, raw); err != nil {
		t.Fatal(err)
	}
	if err := MigrateStore(st); err != nil {
		t.Fatalf("plaintext with no key should no-op: %v", err)
	}
}
