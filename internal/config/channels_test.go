package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestChannelsParsed verifies the optional declarative channels: section is
// parsed into Config.Channels with type/enabled/opaque config preserved.
func TestChannelsParsed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(p, []byte(`
llm:
  provider: mock
channels:
  - type: weixin
    enabled: true
    config:
      creds_dir: ./custom/weixin
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Channels) != 1 {
		t.Fatalf("channels len=%d want 1", len(cfg.Channels))
	}
	ch := cfg.Channels[0]
	if ch.Type != "weixin" {
		t.Fatalf("type=%q want weixin", ch.Type)
	}
	if !ch.Enabled {
		t.Fatal("enabled=false want true")
	}
	if ch.Config["creds_dir"] != "./custom/weixin" {
		t.Fatalf("creds_dir=%q want ./custom/weixin", ch.Config["creds_dir"])
	}
}

// TestChannelsOmittedByDefault verifies a config without a channels: section
// yields an empty slice (back-compat: bootstrap then wires all registered
// channels).
func TestChannelsOmittedByDefault(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(p, []byte("llm:\n  provider: mock\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Channels) != 0 {
		t.Fatalf("channels len=%d want 0 when section omitted", len(cfg.Channels))
	}
}

// TestChannelConfigNameParsed verifies the per-instance Name field parses and
// survives a Load round-trip.
func TestChannelConfigNameParsed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(p, []byte(`
llm:
  provider: mock
channels:
  - name: feishu
    type: webhook
    enabled: true
    config:
      source: feishu
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Channels) != 1 {
		t.Fatalf("channels len=%d want 1", len(cfg.Channels))
	}
	ch := cfg.Channels[0]
	if ch.Name != "feishu" || ch.Type != "webhook" || !ch.Enabled {
		t.Fatalf("unexpected: %+v", ch)
	}
	if ch.Config["source"] != "feishu" {
		t.Fatalf("source=%q", ch.Config["source"])
	}
}
