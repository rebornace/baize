package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/store"
)

// resetCredentialsInStore clears the credential override in the runtime_settings
// KV while preserving any engine-knob override. After this (and a restart or TTL
// refresh) the control plane falls back to the YAML/env break-glass tokens.
func resetCredentialsInStore(ctx context.Context, st store.Store) error {
	raw, ok, err := st.GetSetting(store.SettingKeyRuntimeSettings)
	if err != nil {
		return err
	}
	persisted := struct {
		Knobs json.RawMessage `json:"knobs"`
		Creds json.RawMessage `json:"creds"`
	}{}
	if ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &persisted); err != nil {
			// Corrupt blob: the runtime holder already ignores it wholesale
			// (falls back to the YAML baseline, so break-glass tokens are in
			// effect). Refuse to overwrite it — knobs overrides could be lost.
			return fmt.Errorf("runtime_settings KV is corrupt; refusing to overwrite (engine-knob overrides would be lost); inspect or repair the setting manually: %w", err)
		}
	}
	out := map[string]any{"creds": map[string]any{}}
	if len(persisted.Knobs) > 0 && string(persisted.Knobs) != "null" {
		out["knobs"] = persisted.Knobs
	}
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return st.UpsertSetting(store.SettingKeyRuntimeSettings, b)
}

// runResetCredentials implements `baize reset-credentials -config <path>`.
func runResetCredentials(args []string) error {
	fs := flag.NewFlagSet("reset-credentials", flag.ContinueOnError)
	cfgPath := fs.String("config", startConfigPath(), "path to config yaml (for store driver/path)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	st, err := store.OpenWithOptions(cfg.Store.Driver, store.OpenOptions{
		SQLitePath: cfg.Store.SQLitePath,
		DSN:        cfg.Store.DSN,
	})
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	if c, ok := st.(interface{ Close() error }); ok {
		defer c.Close()
	}
	if err := resetCredentialsInStore(context.Background(), st); err != nil {
		return fmt.Errorf("reset credentials: %w", err)
	}
	log.Printf("control-plane credential overrides cleared; restart or wait for TTL refresh to use YAML tokens")
	return nil
}
