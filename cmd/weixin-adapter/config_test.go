package main

import (
	"testing"
)

func TestParseFlagsDefaults(t *testing.T) {
	cfg, err := parseFlags([]string{"-baize=http://127.0.0.1:8080/v0/channels/weixin/inbound", "-secret=s"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaizeInboundURL == "" || cfg.Secret != "s" {
		t.Fatalf("cfg=%+v", cfg)
	}
	if cfg.Addr == "" {
		t.Fatal("addr default empty")
	}
	if cfg.CredsDir == "" {
		t.Fatal("creds dir default empty")
	}
}

func TestParseFlagsRequiresSecretAndBaize(t *testing.T) {
	if _, err := parseFlags([]string{"-baize=x"}); err == nil {
		t.Fatal("expected error for missing secret")
	}
	if _, err := parseFlags([]string{"-secret=s"}); err == nil {
		t.Fatal("expected error for missing baize url")
	}
}
