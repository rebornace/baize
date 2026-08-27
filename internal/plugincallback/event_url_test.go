package plugincallback_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/plugincallback"
)

func TestEventURLIssuesWhenConfigured(t *testing.T) {
	url := plugincallback.EventURL(plugincallback.Issue, []byte("secret"), "https://runtime.example", time.Hour, "run_42")
	if url == "" {
		t.Fatal("want non-empty URL")
	}
	if !strings.HasPrefix(url, "https://runtime.example/v0/runs/run_42/plugin-callbacks?token=") {
		t.Fatalf("url=%q", url)
	}
}

func TestEventURLEmptyWhenMissingRunID(t *testing.T) {
	url := plugincallback.EventURL(plugincallback.Issue, []byte("secret"), "https://runtime.example", time.Hour, "")
	if url != "" {
		t.Fatalf("url=%q", url)
	}
}

func TestEventURLEmptyWhenMissingPublicBase(t *testing.T) {
	url := plugincallback.EventURL(plugincallback.Issue, []byte("secret"), "", time.Hour, "run_42")
	if url != "" {
		t.Fatalf("url=%q", url)
	}
}
