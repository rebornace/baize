package memory_test

import (
	"context"
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
