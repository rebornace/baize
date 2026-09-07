package webhook

import "testing"

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig("feishu", map[string]string{
		"source":          "feishu",
		"account":         "feishu-bot-1",
		"secret":          "s3cr3t",
		"outbound_secret": "out-s",
		"outbound_url":    "http://adapter:8080/outbound",
		"assignee":        "u-admin",
		"agent_id":        "agent-x",
		"supports_vision": "true",
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
	if !cfg.SupportsVision {
		t.Fatal("supports_vision should parse true")
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
	// invalid supports_vision => error
	if _, err := parseConfig("n", map[string]string{
		"secret":          "s",
		"outbound_url":    "http://x/o",
		"assignee":        "a",
		"supports_vision": "yes",
	}); err == nil {
		t.Fatal("expected error for invalid supports_vision")
	}
}

func TestParseConfigAutostartAllowsEmptySecret(t *testing.T) {
	c, err := parseConfig("weixin", map[string]string{
		"source":            "weixin",
		"outbound_url":      "http://127.0.0.1:8090/outbound",
		"assignee":          "channel:weixin",
		"adapter_autostart": "true",
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
