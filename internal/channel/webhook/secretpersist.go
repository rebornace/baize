package webhook

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// secretFileName holds the stable HMAC secret for an autostart adapter, so the
// key survives baize restarts. Without it, each baize boot generated a fresh
// in-memory key while an adapter orphaned by a previous (killed/crashed) baize
// still held the old key — every admin/outbound call then failed signature
// verification (HTTP 401). Persisting under the per-instance settings dir
// (0o700 dir / 0o600 file) makes a restarted baize and a surviving adapter
// converge on the same secret.
const secretFileName = "secret.key"

// resolveSecret makes an auto-generated autostart secret stable across restarts:
// it reuses a previously persisted secret.key when present, otherwise writes
// the freshly generated one. Explicitly configured secrets (secretAuto=false)
// and in-memory instances (no settingsDir, e.g. tests) are left untouched.
func (c *Channel) resolveSecret() {
	if !c.cfg.secretAuto {
		return
	}
	dir := strings.TrimSpace(c.settingsDir)
	if dir == "" {
		return
	}
	path := filepath.Join(dir, secretFileName)
	if data, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(data)); s != "" {
			c.cfg.Secret = s
			c.cfg.OutboundSecret = s
			return
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("webhook %s: persist secret: mkdir: %v", c.cfg.Name, err)
		return
	}
	if err := os.WriteFile(path, []byte(c.cfg.Secret), 0o600); err != nil {
		log.Printf("webhook %s: persist secret: write: %v", c.cfg.Name, err)
	}
}
