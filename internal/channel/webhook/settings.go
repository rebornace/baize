package webhook

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/channel"
)

const settingsFileName = "settings.json"

// loadSettingsFile reads the persisted per-instance settings. A missing file
// returns the zero value (caller overlays config defaults).
func loadSettingsFile(dir string) (channel.ChannelSettings, error) {
	var out channel.ChannelSettings
	data, err := os.ReadFile(filepath.Join(dir, settingsFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

// saveSettingsFile writes settings atomically (tmp + rename) with 0o700 dir
// and 0o600 file permissions.
func saveSettingsFile(dir string, s channel.ChannelSettings) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, settingsFileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// loadSettings overlays persisted settings over config baseline and applies
// them hot to the Runtime/allowlist. Safe to call when settingsDir is empty
// (tests): uses in-memory defaults only.
func (c *Channel) loadSettings() error {
	dir := strings.TrimSpace(c.settingsDir)
	// Config baseline: assignee/agent and the config-file allowlist seed the
	// hot settings; persisted settings overlay (and win over) the baseline.
	baselineAllow := make([]string, 0, len(c.cfg.Allowlist))
	for peer := range c.cfg.Allowlist {
		baselineAllow = append(baselineAllow, peer)
	}
	sort.Strings(baselineAllow)
	st := channel.ChannelSettings{
		Assignee:  c.cfg.Assignee,
		AgentID:   c.cfg.AgentID,
		Enabled:   true,
		Allowlist: baselineAllow,
	}
	if dir != "" {
		data, err := os.ReadFile(filepath.Join(dir, settingsFileName))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			var persisted channel.ChannelSettings
			if err := json.Unmarshal(data, &persisted); err != nil {
				return err
			}
			if strings.TrimSpace(persisted.Assignee) != "" {
				st.Assignee = persisted.Assignee
			}
			if strings.TrimSpace(persisted.AgentID) != "" {
				st.AgentID = persisted.AgentID
			}
			if persisted.Allowlist != nil {
				st.Allowlist = persisted.Allowlist
			}
			// Enabled defaults true; only an explicit persisted false disables.
			var raw map[string]any
			if json.Unmarshal(data, &raw) == nil {
				if v, ok := raw["enabled"].(bool); ok {
					st.Enabled = v
				}
			}
		}
	}
	c.applySettings(st)
	return nil
}

// applySettings updates the in-memory settings, Runtime assignee/agent, and the
// hot allowlist.
func (c *Channel) applySettings(st channel.ChannelSettings) {
	c.settingsMu.Lock()
	c.settings = st
	c.allowlist = toSet(st.Allowlist)
	c.settingsMu.Unlock()
	if c.rt != nil {
		// Hot update under the Runtime's own routeMu: inbound HTTP goroutines
		// read these fields via Runtime.routing(), so direct writes here would
		// race. SetRouting ignores blank values.
		c.rt.SetRouting(st.Assignee, st.AgentID)
	}
}

func toSet(ids []string) map[string]bool {
	set := map[string]bool{}
	for _, id := range ids {
		if v := strings.TrimSpace(id); v != "" {
			set[v] = true
		}
	}
	return set
}

// peerAllowed reports whether peer passes the hot allowlist. Empty = open.
func (c *Channel) peerAllowed(peer string) bool {
	c.settingsMu.RLock()
	defer c.settingsMu.RUnlock()
	if len(c.allowlist) == 0 {
		return true
	}
	return c.allowlist[peer]
}

// GetSettings returns a copy of the current settings.
func (c *Channel) GetSettings() channel.ChannelSettings {
	c.settingsMu.RLock()
	defer c.settingsMu.RUnlock()
	out := c.settings
	if out.Allowlist == nil {
		out.Allowlist = []string{}
	}
	return out
}

// UpdateSettings applies hot, persists (when a dir is set), reconciles the
// adapter enabled state, and returns the reconciled status.
func (c *Channel) UpdateSettings(st channel.ChannelSettings) channel.AdapterStatus {
	if st.Allowlist == nil {
		st.Allowlist = []string{}
	}
	c.applySettings(st)
	if dir := strings.TrimSpace(c.settingsDir); dir != "" {
		if err := saveSettingsFile(dir, st); err != nil {
			log.Printf("webhook %s: save settings: %v", c.cfg.Name, err)
		}
	}
	c.reconcileEnabled(st.Enabled)
	return c.Status()
}

// Status reconciles running/reason from local enabled state and the adapter.
func (c *Channel) Status() channel.AdapterStatus {
	c.settingsMu.RLock()
	enabled := c.settings.Enabled
	manualStop := c.manualStop
	c.settingsMu.RUnlock()
	if !enabled {
		return channel.AdapterStatus{Running: false, Reason: ""}
	}
	if manualStop {
		// Operator explicitly stopped the adapter process; do not probe the
		// dead port and mislabel it "start_failed".
		return channel.AdapterStatus{Running: false, Reason: "stopped"}
	}
	if c.sup != nil && c.sup.restarting() {
		// Watchdog is parked in a backoff/respawn loop after a crash.
		return channel.AdapterStatus{Running: false, Reason: "restarting"}
	}
	if c.admin == nil {
		// No adapter management plane: nothing to probe; report running when
		// enabled (third-party adapters manage their own lifecycle).
		return channel.AdapterStatus{Running: true}
	}
	hasCreds, polling, err := c.admin.Status(c.bgCtx())
	if err != nil {
		log.Printf("webhook %s: adapter status: %v", c.cfg.Name, err)
		return channel.AdapterStatus{Running: false, Reason: "start_failed"}
	}
	if !hasCreds {
		return channel.AdapterStatus{Running: false, Reason: "login_required"}
	}
	if !polling {
		return channel.AdapterStatus{Running: false, Reason: "start_failed"}
	}
	return channel.AdapterStatus{Running: true}
}

// reconcileEnabled starts/stops the adapter polling to match enabled. Errors
// are logged only: Status() surfaces the resulting adapter state.
func (c *Channel) reconcileEnabled(enabled bool) {
	if c.admin == nil {
		return
	}
	ctx := c.bgCtx()
	if enabled {
		if err := c.admin.Start(ctx); err != nil {
			log.Printf("webhook %s: adapter start: %v", c.cfg.Name, err)
		}
	} else {
		if err := c.admin.Stop(ctx); err != nil {
			log.Printf("webhook %s: adapter stop: %v", c.cfg.Name, err)
		}
	}
}
