package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
)

// tokenRecordingLink wraps the Fake and records the token each GetUpdates call
// runs with, blocking briefly so a long-running loop is observable.
type tokenRecordingLink struct {
	weixinlink.ILink
	mu     sync.Mutex
	tokens []string
}

func (l *tokenRecordingLink) GetUpdates(ctx context.Context, token, cursor string) ([]weixinlink.Update, string, error) {
	l.mu.Lock()
	l.tokens = append(l.tokens, token)
	l.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	case <-time.After(15 * time.Millisecond):
	}
	return nil, "", nil
}

func (l *tokenRecordingLink) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.tokens))
	copy(out, l.tokens)
	return out
}

func waitForToken(t *testing.T, l *tokenRecordingLink, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, tk := range l.snapshot() {
			if tk == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("GetUpdates never ran with token %q (saw %v)", want, l.snapshot())
}

// TestStartPollingRestartsOnTokenChange: after a re-scan replaces the token
// while a poll loop is running, startPolling must restart the loop bound to the
// new token. Previously it no-op'd when polling==true, leaving the stale loop
// to fail GetUpdates forever.
func TestStartPollingRestartsOnTokenChange(t *testing.T) {
	link := &tokenRecordingLink{ILink: weixinlink.NewFake()}
	a := &Adapter{
		ilink:     link,
		secret:    "s",
		emptyWait: 1 * time.Millisecond,
	}

	a.setCredentials("acct", "tok-old")
	if err := a.startPolling(); err != nil {
		t.Fatalf("startPolling old: %v", err)
	}
	waitForToken(t, link, "tok-old", time.Second)

	// Simulate a fresh QR login: new credential while the old loop runs.
	a.setCredentials("acct", "tok-new")
	if err := a.startPolling(); err != nil {
		t.Fatalf("startPolling new: %v", err)
	}
	waitForToken(t, link, "tok-new", time.Second)

	if !a.isPolling() {
		t.Fatal("should still be polling after restart")
	}
	// Clean shutdown must not hang (the restarted loop must be the tracked one).
	done := make(chan struct{})
	go func() { a.stopPolling(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stopPolling hung after token-change restart")
	}
}

// TestStartPollingIdempotentForSameToken: unchanged token must not restart.
func TestStartPollingIdempotentForSameToken(t *testing.T) {
	link := &tokenRecordingLink{ILink: weixinlink.NewFake()}
	a := &Adapter{ilink: link, secret: "s", emptyWait: 1 * time.Millisecond}
	a.setCredentials("acct", "tok")
	if err := a.startPolling(); err != nil {
		t.Fatalf("startPolling: %v", err)
	}
	if err := a.startPolling(); err != nil {
		t.Fatalf("idempotent startPolling: %v", err)
	}
	waitForToken(t, link, "tok", time.Second)
	a.stopPolling()
}
