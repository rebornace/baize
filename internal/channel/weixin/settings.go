package weixin

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const settingsFileName = "settings.json"

// DefaultCredsDir is the default on-disk location for weixin creds + settings.
const DefaultCredsDir = "./data/channels/weixin"

// Settings is persisted next to weixin creds as settings.json. The JSON field
// names are the public API contract consumed by the settings UI and must not
// change.
type Settings struct {
	AgentID   string   `json:"agent_id"`
	Allowlist []string `json:"allowlist"`
	Assignee  string   `json:"assignee"`
	Enabled   bool     `json:"enabled"`
}

// LoadSettings reads settings.json from dir. When the file is missing the
// channel defaults to enabled with an empty (non-nil) allowlist.
func LoadSettings(dir string) (Settings, error) {
	path := filepath.Join(dir, settingsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{Allowlist: []string{}, Enabled: true}, nil
		}
		return Settings{}, err
	}
	var out Settings
	if err := json.Unmarshal(data, &out); err != nil {
		return Settings{}, err
	}
	if out.Allowlist == nil {
		out.Allowlist = []string{}
	}
	return out, nil
}

// SaveSettings writes settings.json under dir atomically (tmp + rename),
// creating the directory with 0700 perms when needed.
func SaveSettings(dir string, settings Settings) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, settingsFileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
