package weixin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const credsFileName = "creds.json"

type credsPayload struct {
	AccountID string `json:"account_id"`
	Token     string `json:"token"`
}

// SaveCreds writes accountID and token under dir (mkdir 0700 when possible).
func SaveCreds(dir, accountID, token string) error {
	if dir == "" {
		return fmt.Errorf("weixin creds: empty dir")
	}
	if accountID == "" || token == "" {
		return fmt.Errorf("weixin creds: account_id and token are required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("weixin creds: mkdir: %w", err)
	}
	payload, err := json.MarshalIndent(credsPayload{AccountID: accountID, Token: token}, "", "  ")
	if err != nil {
		return fmt.Errorf("weixin creds: marshal: %w", err)
	}
	path := filepath.Join(dir, credsFileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return fmt.Errorf("weixin creds: write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("weixin creds: rename: %w", err)
	}
	return nil
}

// LoadCreds reads accountID and token from dir/creds.json.
func LoadCreds(dir string) (accountID, token string, err error) {
	if dir == "" {
		return "", "", fmt.Errorf("weixin creds: empty dir")
	}
	path := filepath.Join(dir, credsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("weixin creds: read: %w", err)
	}
	var p credsPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return "", "", fmt.Errorf("weixin creds: unmarshal: %w", err)
	}
	if p.AccountID == "" || p.Token == "" {
		return "", "", fmt.Errorf("weixin creds: missing account_id or token")
	}
	return p.AccountID, p.Token, nil
}

// ClearCreds removes dir/creds.json if present (idempotent).
func ClearCreds(dir string) error {
	if dir == "" {
		return fmt.Errorf("weixin creds: empty dir")
	}
	path := filepath.Join(dir, credsFileName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("weixin creds: remove: %w", err)
	}
	return nil
}
