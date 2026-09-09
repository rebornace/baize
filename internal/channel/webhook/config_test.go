package webhook

import (
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig("feishu", map[string]string{
		"source":          "feishu",
		"account":         "feishu-bot-1",
		"secret":          "s3cr3t",
		"outbound_secret": "out-s",
		"outbound_url":    "http://adapter:8080/outbound",
		"assignee":        "u-admin",
		"agent_id":        "agent-x",
		"allowlist":       "u1, u2 ,,u3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source != "feishu" || cfg.Account != "feishu-bot-1" || cfg.Secret != "s3cr3t" {
		t.Fatalf("bad base fields: %+v", cfg)
	}
	if cfg.OutboundSecret != "out-s" {
		t.Fatalf("explicit outbound_secret should win, got %q", cfg.OutboundSecret)
	}
	if cfg.OutboundURL != "http://adapter:8080/outbound" || cfg.Assignee != "u-admin" || cfg.AgentID != "agent-x" {
		t.Fatalf("bad wiring fields: %+v", cfg)
	}
	if len(cfg.Allowlist) != 3 || cfg.Allowlist["u1"] == false || cfg.Allowlist["u3"] == false {
		t.Fatalf("bad allowlist: %+v", cfg.Allowlist)
	}
}

func TestParseConfigDefaultsAndErrors(t *testing.T) {
	// source/account default to instance name (passed via name arg)
	cfg, err := parseConfig("feishu", map[string]string{"secret": "s", "outbound_url": "http://x/o", "assignee": "a"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source != "feishu" || cfg.Account != "feishu" {
		t.Fatalf("expected source/account default to instance name, got %+v", cfg)
	}
	// outbound_secret defaults to secret when not set
	if cfg.OutboundSecret != "s" {
		t.Fatalf("expected outbound_secret default to secret %q, got %q", "s", cfg.OutboundSecret)
	}
	// missing secret => error
	if _, err := parseConfig("n", map[string]string{"outbound_url": "http://x/o", "assignee": "a"}); err == nil {
		t.Fatal("expected error for missing secret")
	}
	// missing outbound_url => error
	if _, err := parseConfig("n", map[string]string{"secret": "s", "assignee": "a"}); err == nil {
		t.Fatal("expected error for missing outbound_url")
	}
	// missing assignee => error
	if _, err := parseConfig("n", map[string]string{"secret": "s", "outbound_url": "http://x/o"}); err == nil {
		t.Fatal("expected error for missing assignee")
	}
}

func TestParseConfigAutostartAllowsEmptySecret(t *testing.T) {
	c, err := parseConfig("weixin", map[string]string{
		"source":            "weixin",
		"outbound_url":      "http://127.0.0.1:8090/outbound",
		"assignee":          "channel:weixin",
		"adapter_autostart": "true",
		"adapter_command":   "weixin-adapter",
		"admin_url":         "http://127.0.0.1:8090",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !c.AdapterAutostart {
		t.Fatal("AdapterAutostart not parsed")
	}
	if c.Secret == "" {
		t.Fatal("secret should be generated for autostart")
	}
	if c.OutboundSecret != c.Secret {
		t.Fatal("outbound secret should fall back to generated secret")
	}
}

func TestParseConfigAdminAndAdapterFields(t *testing.T) {
	c, err := parseConfig("bot", map[string]string{
		"secret":          "s",
		"outbound_url":    "http://h/o",
		"assignee":        "a",
		"admin_url":       "http://127.0.0.1:8090",
		"adapter_command": "weixin-adapter",
		"adapter_args":    "-addr=127.0.0.1:8090,-creds=./data/channels/weixin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.AdminURL != "http://127.0.0.1:8090" || c.AdapterCommand != "weixin-adapter" {
		t.Fatalf("admin/cmd = %q %q", c.AdminURL, c.AdapterCommand)
	}
	if len(c.AdapterArgs) != 2 || c.AdapterArgs[0] != "-addr=127.0.0.1:8090" {
		t.Fatalf("adapter args = %v", c.AdapterArgs)
	}
}

func TestParseConfigSecretRequiredWithoutAutostart(t *testing.T) {
	if _, err := parseConfig("x", map[string]string{"outbound_url": "http://h/o", "assignee": "a"}); err == nil {
		t.Fatal("expected error for missing secret when not autostart")
	}
}

func TestParseConfigAutostartRequiresCommandAndAdminURL(t *testing.T) {
	base := map[string]string{
		"outbound_url":      "http://h/o",
		"assignee":          "a",
		"adapter_autostart": "true",
	}
	// autostart without adapter_command => error.
	missCmd := map[string]string{}
	for k, v := range base {
		missCmd[k] = v
	}
	missCmd["admin_url"] = "http://127.0.0.1:8090"
	if _, err := parseConfig("wx", missCmd); err == nil {
		t.Fatal("expected error for autostart without adapter_command")
	} else if !strings.Contains(err.Error(), "adapter_command") {
		t.Fatalf("error should mention adapter_command, got: %v", err)
	}
	// autostart without admin_url => error.
	missAdmin := map[string]string{}
	for k, v := range base {
		missAdmin[k] = v
	}
	missAdmin["adapter_command"] = "weixin-adapter"
	if _, err := parseConfig("wx", missAdmin); err == nil {
		t.Fatal("expected error for autostart without admin_url")
	} else if !strings.Contains(err.Error(), "admin_url") {
		t.Fatalf("error should mention admin_url, got: %v", err)
	}
	// autostart with both present => OK (secret auto-generated).
	ok := map[string]string{}
	for k, v := range base {
		ok[k] = v
	}
	ok["adapter_command"] = "weixin-adapter"
	ok["admin_url"] = "http://127.0.0.1:8090"
	c, err := parseConfig("wx", ok)
	if err != nil {
		t.Fatalf("autostart with command+admin_url should parse: %v", err)
	}
	if c.AdapterBaizeURL != "" {
		t.Fatalf("AdapterBaizeURL should default empty, got %q", c.AdapterBaizeURL)
	}
	// adapter_baize_url parses through.
	ok["adapter_baize_url"] = "http://127.0.0.1:9000"
	c, err = parseConfig("wx", ok)
	if err != nil {
		t.Fatal(err)
	}
	if c.AdapterBaizeURL != "http://127.0.0.1:9000" {
		t.Fatalf("AdapterBaizeURL = %q", c.AdapterBaizeURL)
	}
}
