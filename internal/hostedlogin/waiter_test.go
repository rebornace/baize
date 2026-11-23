package hostedlogin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/hostedlogin"
)

func TestWaiterNotifyOK(t *testing.T) {
	w := hostedlogin.NewMemoryWaiter()
	done := make(chan error, 1)
	go func() {
		done <- w.Await(context.Background(), "s1")
	}()
	time.Sleep(20 * time.Millisecond)
	w.NotifyOK("s1")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Await: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestWaiterNotifyFail(t *testing.T) {
	w := hostedlogin.NewMemoryWaiter()
	done := make(chan error, 1)
	go func() {
		done <- w.Await(context.Background(), "s2")
	}()
	time.Sleep(20 * time.Millisecond)
	want := errors.New("boom")
	w.NotifyFail("s2", want)
	select {
	case err := <-done:
		if !errors.Is(err, want) && err.Error() != want.Error() {
			t.Fatalf("got %v want %v", err, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestWaiterContextCancel(t *testing.T) {
	w := hostedlogin.NewMemoryWaiter()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- w.Await(ctx, "s3")
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}
