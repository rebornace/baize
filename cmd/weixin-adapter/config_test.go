package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
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

func TestParseFlagsPortFile(t *testing.T) {
	cfg, err := parseFlags([]string{
		"-baize=http://127.0.0.1:8080/inbound",
		"-secret=s",
		"-addr=127.0.0.1:0",
		"-port-file=/tmp/p",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PortFile != "/tmp/p" {
		t.Fatalf("PortFile=%q", cfg.PortFile)
	}
}

func TestWritePortFileAfterListen(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	path := filepath.Join(t.TempDir(), "sub", "listen.port")
	addr := ln.Addr().String()
	if err := writeListenPortFile(path, addr); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(raw))
	if got != addr {
		t.Fatalf("got %q want %q", got, addr)
	}
	if _, _, err := net.SplitHostPort(got); err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
}
