package main

import (
	"errors"
	"flag"
	"strings"
)

// Config is the adapter process configuration, supplied via command-line
// flags by baize (autostart) or by an operator (standalone deployment).
type Config struct {
	BaizeInboundURL string // baize inbound webhook to POST messages to
	Secret          string // shared HMAC secret with baize
	Addr            string // adapter listen address (admin/outbound/healthz)
	CredsDir        string // iLink creds.json directory (default ./data/channels/weixin)
	ILinkBaseURL    string // override iLink API base (tests)
}

func parseFlags(args []string) (Config, error) {
	fs := flag.NewFlagSet("weixin-adapter", flag.ContinueOnError)
	var cfg Config
	fs.StringVar(&cfg.BaizeInboundURL, "baize", "", "baize inbound webhook URL")
	fs.StringVar(&cfg.Secret, "secret", "", "shared HMAC secret")
	fs.StringVar(&cfg.Addr, "addr", "127.0.0.1:8090", "listen address")
	fs.StringVar(&cfg.CredsDir, "creds", "./data/channels/weixin", "creds directory")
	fs.StringVar(&cfg.ILinkBaseURL, "ilink-base", "", "iLink API base URL override")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if strings.TrimSpace(cfg.Secret) == "" {
		return cfg, errors.New("weixin-adapter: -secret is required")
	}
	if strings.TrimSpace(cfg.BaizeInboundURL) == "" {
		return cfg, errors.New("weixin-adapter: -baize (inbound url) is required")
	}
	return cfg, nil
}
