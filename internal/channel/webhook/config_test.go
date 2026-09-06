package webhook

import "testing"

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig("feishu", map[string]string{
		"source":          "feishu",
		"account":         "feishu-bot-1",
		"secret":          "s3cr3t",
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
