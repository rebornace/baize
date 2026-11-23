package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
)

// scriptedPollLink returns a session-timeout error while failSession is true;
// otherwise it delegates to the embedded Fake. It counts GetUpdates calls so a
// test can prove the loop stops instead of retrying forever.
type scriptedPollLink struct {
	weixinlink.ILink
	mu          sync.Mutex
	calls       int
	failSession bool
}

func (l *scriptedPollLink) GetUpdates(ctx context.Context, token, cursor string) ([]weixinlink.Update, string, error) {
	l.mu.Lock()
	l.calls++
	fail := l.failSession
	l.mu.Unlock()
	if fail {
		return nil, "", &weixinlink.SessionError{ErrCode: weixinlink.ErrCodeSessionTimeout, ErrMsg: "session timeout"}
	}
	return l.ILink.GetUpdates(ctx, token, cursor)
}

func (l *scriptedPollLink) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

func (l *scriptedPollLink) setFail(v bool) {
	l.mu.Lock()
	l.failSession = v
	l.mu.Unlock()
}

// waitForPolling polls until the adapter's polling flag equals want.
func waitForPolling(a *Adapter, want bool, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if a.isPolling() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (a *Adapter) isLoginExpired() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.loginExpired
}

// TestPollLoopStopsOnSessionTimeout: errcode -14 must terminate the poll loop
// after exactly one attempt and mark the session expired — not retry forever.
func TestPollLoopStopsOnSessionTimeout(t *testing.T) {
	link := &scriptedPollLink{ILink: weixinlink.NewFake(), failSession: true}
	a := &Adapter{ilink: link, secret: "s", emptyWait: 1 * time.Millisecond}

	a.setCredentials("acct", "tok")
	if err := a.startPolling(); err != nil {
		t.Fatalf("startPolling: %v", err)
	}

	waitForPolling(a, false, time.Second)
	if a.isPolling() {
		t.Fatal("polling should stop after session timeout")
	}
	if !a.isLoginExpired() {
		t.Fatal("loginExpired should be true after session timeout")
	}
	// Give any (buggy) retry a chance to run, then ensure it never did.
	time.Sleep(100 * time.Millisecond)
	if got := link.callCount(); got != 1 {
		t.Fatalf("GetUpdates called %d times; want exactly 1 (no retry)", got)
	}
}

// TestReScanClearsExpiredAndResumes: after a fresh QR login, startPolling must
// clear the expired flag and resume polling with a healthy link.
func TestReScanClearsExpiredAndResumes(t *testing.T) {
	link := &scriptedPollLink{ILink: weixinlink.NewFake(), failSession: true}
	a := &Adapter{ilink: link, secret: "s", emptyWait: 1 * time.Millisecond}

	a.setCredentials("acct", "tok-old")
	if err := a.startPolling(); err != nil {
		t.Fatalf("startPolling old: %v", err)
	}
	waitForPolling(a, false, time.Second)
	if !a.isLoginExpired() {
		t.Fatal("precondition: session should be expired")
	}

	// New credential from a fresh scan; the link is healthy again.
	link.setFail(false)
	a.setCredentials("acct", "tok-new")
	if err := a.startPolling(); err != nil {
		t.Fatalf("startPolling new: %v", err)
	}

	waitForPolling(a, true, time.Second)
	if !a.isPolling() {
		t.Fatal("polling should resume after re-scan")
	}
	if a.isLoginExpired() {
		t.Fatal("loginExpired must be cleared after re-scan")
	}
	a.stopPolling()
}
