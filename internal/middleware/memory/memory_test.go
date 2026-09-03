package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/middleware"
	_ "github.com/rebornace/baize/internal/middleware/memory"
)

func TestMemoryQueueEnqueueConsumeAck(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()

	job := middleware.Job{RunID: "run_1", Kind: middleware.KindRun, Input: "hi", EnqueuedAt: time.Now()}
	if err := mw.Queue.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, ack, ok := mw.Queue.Consume(ctx)
	if !ok {
		t.Fatal("expected to consume a job")
	}
	if got.RunID != "run_1" {
		t.Fatalf("got run %q", got.RunID)
	}
	ack()
}

func TestMemoryQueueEnqueueFillsEnqueuedAt(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()

	job := middleware.Job{RunID: "run_ts", Kind: middleware.KindRun, Input: "hi"}
	if err := mw.Queue.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, _, ok := mw.Queue.Consume(ctx)
	if !ok {
		t.Fatal("expected to consume a job")
	}
	if got.EnqueuedAt.IsZero() {
		t.Fatal("zero EnqueuedAt must be backfilled")
	}
}

func TestMemoryQueueCloseIdempotentAndEnqueueErrors(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{})
	if err != nil {
		t.Fatal(err)
	}

	if err := mw.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("repeat close must be idempotent: %v", err)
	}

	// Enqueue after close must return an error, not panic.
	if err := mw.Queue.Enqueue(context.Background(), middleware.Job{RunID: "run_dead", Kind: middleware.KindRun}); err == nil {
		t.Fatal("enqueue after close must error")
	}

	// Consume after close must return ok=false immediately.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, ok := mw.Queue.Consume(ctx); ok {
		t.Fatal("consume after close must return ok=false")
	}
}

func TestMemoryQueueEnqueueRespectsContext(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()

	// A single consumer drains nothing: fill the 1024-buffer so Enqueue blocks.
	ctxFill, cancelFill := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFill()
	for i := 0; i < 1024; i++ {
		if err := mw.Queue.Enqueue(ctxFill, middleware.Job{RunID: "run_fill", Kind: middleware.KindRun}); err != nil {
			t.Fatalf("fill %d: %v", i, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- mw.Queue.Enqueue(ctx, middleware.Job{RunID: "run_blocked", Kind: middleware.KindRun})
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue did not return after ctx cancel")
	}
}

func TestMemoryBusLocalNudge(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()
	ch, err := mw.Bus.SubscribeRunEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := mw.Bus.PublishRunEvent(context.Background(), "run_x", 7); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-ch:
		if n.RunID != "run_x" || n.Seq != 7 {
			t.Fatalf("bad nudge %+v", n)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive local nudge")
	}
}

func TestMemoryLimiterWindow(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()
	// memory limiter default window; hammer one key and assert it eventually denies.
	allowed := 0
	for i := 0; i < 5000; i++ {
		if mw.Limiter.Allow("k") {
			allowed++
		}
	}
	if allowed == 0 || allowed >= 5000 {
		t.Fatalf("limiter should allow a bounded budget, allowed=%d", allowed)
	}
}

// 确保 Hub 可被总线驱动（memory bus 包一个 *eventbus.Hub）。
var _ = eventbus.NewHub
