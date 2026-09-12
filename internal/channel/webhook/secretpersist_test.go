package webhook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSecretPersistsAndReuses(t *testing.T) {
	dir := t.TempDir()

	// First boot: generated secret is persisted to secret.key.
	first := &Channel{
		cfg:         instanceConfig{Name: "weixin", Secret: "generated-1", OutboundSecret: "generated-1", secretAuto: true},
		settingsDir: filepath.Join(dir, "channels", "webhook", "weixin"),
	}
	first.resolveSecret()
	if first.cfg.Secret != "generated-1" {
		t.Fatalf("first secret changed: %q", first.cfg.Secret)
	}
	data, err := os.ReadFile(filepath.Join(first.settingsDir, secretFileName))
	if err != nil {
		t.Fatalf("secret.key not persisted: %v", err)
	}
	if string(data) != "generated-1" {
		t.Fatalf("persisted secret = %q want generated-1", string(data))
	}

	// Second boot (simulating a restart): a freshly generated in-memory secret
	// must be replaced by the persisted one so an orphaned adapter (holding the
	// old secret) still verifies baize's requests.
	second := &Channel{
		cfg:         instanceConfig{Name: "weixin", Secret: "brand-new-2", OutboundSecret: "brand-new-2", secretAuto: true},
		settingsDir: first.settingsDir,
	}
	second.resolveSecret()
	if second.cfg.Secret != "generated-1" || second.cfg.OutboundSecret != "generated-1" {
		t.Fatalf("restart should reuse persisted secret, got %q/%q", second.cfg.Secret, second.cfg.OutboundSecret)
	}
}

func TestResolveSecretLeavesExplicitSecretUntouched(t *testing.T) {
	dir := t.TempDir()
	c := &Channel{
		cfg:         instanceConfig{Name: "weixin", Secret: "operator-configured", secretAuto: false},
		settingsDir: dir,
	}
	c.resolveSecret()
	if c.cfg.Secret != "operator-configured" {
		t.Fatalf("explicit secret must not be overwritten: %q", c.cfg.Secret)
	}
	if _, err := os.Stat(filepath.Join(dir, secretFileName)); !os.IsNotExist(err) {
		t.Fatalf("no secret.key should be written for explicit secrets, stat err=%v", err)
	}
}

func TestResolveSecretNoopWithoutDir(t *testing.T) {
	// In-memory instances (tests): no dir → generated secret stays as-is, no crash.
	c := &Channel{cfg: instanceConfig{Name: "x", Secret: "ephemeral", secretAuto: true}}
	c.resolveSecret()
	if c.cfg.Secret != "ephemeral" {
		t.Fatalf("ephemeral secret changed: %q", c.cfg.Secret)
	}
}
