package run

import (
	"context"
	"testing"
	"time"
)

func TestGateWaitResumeApprove(t *testing.T) {
	g := NewGate()
	ctx := context.Background()

	errCh := make(chan error, 1)
	decCh := make(chan Decision, 1)
	go func() {
		d, err := g.Wait(ctx, "run1")
		errCh <- err
		decCh <- d
	}()

	time.Sleep(50 * time.Millisecond)
	if err := g.Resume("run1", Decision{Approve: true, Comment: "ok"}); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Wait: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait timed out")
	}
	d := <-decCh
	if !d.Approve || d.Comment != "ok" {
		t.Fatalf("decision=%+v", d)
	}
}

func TestGateWaitResumeReject(t *testing.T) {
	g := NewGate()
	ctx := context.Background()

	done := make(chan Decision, 1)
	go func() {
		d, err := g.Wait(ctx, "run2")
		if err != nil {
			t.Errorf("Wait: %v", err)
			return
		}
		done <- d
	}()

	time.Sleep(50 * time.Millisecond)
	if err := g.Resume("run2", Decision{Approve: false, Comment: "no"}); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	select {
	case d := <-done:
		if d.Approve {
			t.Fatalf("want reject, got %+v", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait timed out")
	}
}

func TestGateResumeNotWaiting(t *testing.T) {
	g := NewGate()
	if err := g.Resume("missing", Decision{Approve: true}); err == nil {
		t.Fatal("expected error when not waiting")
	}
}
