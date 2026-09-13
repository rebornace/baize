package store

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/settingscrypto"
)

type migrateRuntimeSettings struct {
	Knobs json.RawMessage `json:"knobs,omitempty"`
	Creds migrateCreds    `json:"creds,omitempty"`
}

type migrateCreds struct {
	OperatorToken string            `json:"operator_token,omitempty"`
	AdminToken    string            `json:"admin_token,omitempty"`
	Operators     []migrateOperator `json:"operators,omitempty"`
}

type migrateOperator struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

// MigrateStore encrypts plaintext secrets in K1 settings (model profiles,
// runtime_settings creds, inbox channel secrets) when BAIZE_SETTINGS_KEY is set.
// Without a key, it is a no-op unless sealed values exist, in which case it
// returns settingscrypto.ErrCiphertext so bootstrap can refuse startup.
func MigrateStore(st Store) error {
	if st == nil {
		return nil
	}
	key, err := settingscrypto.KeyFromEnv()
	if err != nil {
		return err
	}
	if len(key) == 0 {
		return migrateRejectSealedWithoutKey(st)
	}
	return migrateSealPlaintext(st, key)
}

func migrateRejectSealedWithoutKey(st Store) error {
	if _, err := st.ListModelProfiles(); err != nil {
		if errors.Is(err, settingscrypto.ErrCiphertext) {
			return settingscrypto.ErrCiphertext
		}
		return err
	}
	raw, ok, err := st.GetSetting(SettingKeyRuntimeSettings)
	if err != nil {
		return err
	}
	if ok && len(raw) > 0 {
		if err := runtimeSettingsHasSealed(raw); err != nil {
			return err
		}
	}
	raw, ok, err = st.GetSetting(SettingKeyInboxChannels)
	if err != nil {
		return err
	}
	if ok && len(raw) > 0 {
		if err := inboxChannelsHasSealed(raw); err != nil {
			return err
		}
	}
	return nil
}

func runtimeSettingsHasSealed(raw []byte) error {
	var p migrateRuntimeSettings
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil
	}
	if credsHasSealed(&p.Creds) {
		return settingscrypto.ErrCiphertext
	}
	return nil
}

func credsHasSealed(c *migrateCreds) bool {
	if c == nil {
		return false
	}
	if settingscrypto.IsSealed(c.OperatorToken) || settingscrypto.IsSealed(c.AdminToken) {
		return true
	}
	for _, op := range c.Operators {
		if settingscrypto.IsSealed(op.Token) {
			return true
		}
	}
	return false
}

func inboxChannelsHasSealed(raw []byte) error {
	var channels []inbox.Channel
	if err := json.Unmarshal(raw, &channels); err != nil {
		return nil
	}
	for _, c := range channels {
		if settingscrypto.IsSealed(c.Secret) {
			return settingscrypto.ErrCiphertext
		}
	}
	return nil
}

func migrateSealPlaintext(st Store, key settingscrypto.Key) error {
	if err := migrateModelProfiles(st, key); err != nil {
		return err
	}

	raw, ok, err := st.GetSetting(SettingKeyRuntimeSettings)
	if err != nil {
		return err
	}
	if ok && len(raw) > 0 {
		if err := migrateRuntimeSettingsKV(st, key, raw); err != nil {
			return err
		}
	}

	raw, ok, err = st.GetSetting(SettingKeyInboxChannels)
	if err != nil {
		return err
	}
	if ok && len(raw) > 0 {
		if err := migrateInboxChannelsKV(st, key, raw); err != nil {
			return err
		}
	}
	return nil
}

func migrateModelProfiles(st Store, key settingscrypto.Key) error {
	switch s := st.(type) {
	case *Memory:
		s.mu.RLock()
		snap := make([]ModelProfile, 0, len(s.modelProfiles))
		for _, p := range s.modelProfiles {
			snap = append(snap, p)
		}
		s.mu.RUnlock()
		for _, p := range snap {
			stored := p.APIKey
			if stored == "" || settingscrypto.IsSealed(stored) {
				continue
			}
			p.APIKey = stored
			if _, err := st.UpsertModelProfile(p); err != nil {
				return err
			}
		}
		return nil
	case *SQLStore:
		rows, err := s.query(`SELECT id, ` + upsertModelProfileColumns + ` FROM model_profiles`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanModelProfileStored(rows)
			if err != nil {
				return err
			}
			stored := p.APIKey
			if stored == "" || settingscrypto.IsSealed(stored) {
				continue
			}
			if _, err := st.UpsertModelProfile(p); err != nil {
				return err
			}
		}
		return rows.Err()
	default:
		return nil
	}
}

func migrateRuntimeSettingsKV(st Store, key settingscrypto.Key, raw []byte) error {
	var p migrateRuntimeSettings
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil
	}
	changed, err := sealPlainCreds(key, &p.Creds)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	out, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return st.UpsertSetting(SettingKeyRuntimeSettings, out)
}

func sealPlainCreds(key settingscrypto.Key, c *migrateCreds) (changed bool, err error) {
	if c.OperatorToken != "" && !settingscrypto.IsSealed(c.OperatorToken) {
		sealed, err := settingscrypto.Seal(key, c.OperatorToken)
		if err != nil {
			return false, err
		}
		c.OperatorToken = sealed
		changed = true
	}
	if c.AdminToken != "" && !settingscrypto.IsSealed(c.AdminToken) {
		sealed, err := settingscrypto.Seal(key, c.AdminToken)
		if err != nil {
			return false, err
		}
		c.AdminToken = sealed
		changed = true
	}
	for i := range c.Operators {
		tok := c.Operators[i].Token
		if tok == "" || settingscrypto.IsSealed(tok) {
			continue
		}
		sealed, err := settingscrypto.Seal(key, tok)
		if err != nil {
			return false, err
		}
		c.Operators[i].Token = sealed
		changed = true
	}
	return changed, nil
}

func migrateInboxChannelsKV(st Store, key settingscrypto.Key, raw []byte) error {
	var channels []inbox.Channel
	if err := json.Unmarshal(raw, &channels); err != nil {
		return nil
	}
	changed := false
	for i := range channels {
		secret := channels[i].Secret
		if secret == "" || settingscrypto.IsSealed(secret) {
			continue
		}
		sealed, err := settingscrypto.Seal(key, secret)
		if err != nil {
			return err
		}
		channels[i].Secret = sealed
		changed = true
	}
	if !changed {
		return nil
	}
	out, err := json.Marshal(channels)
	if err != nil {
		return err
	}
	if bytes.Equal(out, raw) {
		return nil
	}
	return st.UpsertSetting(SettingKeyInboxChannels, out)
}
