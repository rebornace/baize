package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/middleware"
	mwredis "github.com/rebornace/baize/internal/middleware/redis"
	"github.com/rebornace/baize/internal/store"
	goredis "github.com/redis/go-redis/v9"
)

func job(runID string) middleware.Job {
	return middleware.Job{
		RunID:      runID,
		Kind:       middleware.KindRun,
		Input:      "x",
		EnqueuedAt: time.Now(),
	}
}

func newTestMiddleware(t *testing.T) *middleware.Middleware {
	t.Helper()
	mr := miniredis.RunT(t)
	b, err := mwredis.Open(context.Background(), mwredis.Config{
		Addr: mr.Addr(), Stream: "baize:runs", ConsumerGroup: "baize-workers",
		EventsChannel: "baize:run-events", ConsumerName: "test-consumer",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func TestRedisQueueEnqueueConsumeAck(t *testing.T) {
	b := newTestMiddleware(t)
	ctx := context.Background()
	if err := b.Queue.Enqueue(ctx, job("run_r1")); err != nil {
		t.Fatal(err)
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	got, ack, ok := b.Queue.Consume(cctx)
	if !ok || got.RunID != "run_r1" {
		t.Fatalf("consume: ok=%v job=%+v", ok, got)
	}
	ack()
}

func TestRedisBusPubSub(t *testing.T) {
	b := newTestMiddleware(t)
	ctx := context.Background()
	ch, err := b.Bus.SubscribeRunEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // wait for subscription
	if err := b.Bus.PublishRunEvent(ctx, "run_b1", 5); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-ch:
		if n.RunID != "run_b1" || n.Seq != 5 {
			t.Fatalf("nudge=%+v", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no nudge")
	}
}

func TestRedisLimiterBudget(t *testing.T) {
	b := newTestMiddleware(t)
	allowed := 0
	for i := 0; i < 200; i++ {
		if b.Limiter.Allow("k") {
			allowed++
		}
	}
	if allowed == 0 || allowed > 130 { // default ~120/min budget + small clock skew
		t.Fatalf("allowed=%d", allowed)
	}
}

func TestRedisBusBridgeToHub(t *testing.T) {
	mr := miniredis.RunT(t)
	mw, err := mwredis.Open(context.Background(), mwredis.Config{
		Addr: mr.Addr(), Stream: "baize:runs", ConsumerGroup: "baize-workers",
		EventsChannel: "baize:run-events", ConsumerName: "bridge-consumer",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mw.Close() })

	type hubBridge interface {
		BridgeToHub(hub *eventbus.Hub)
		Start(ctx context.Context)
	}
	b, ok := mw.Bus.(hubBridge)
	if !ok {
		t.Fatal("bus does not implement BridgeToHub/Start")
	}

	hub := eventbus.NewHub()
	sub := hub.Subscribe("run_bridge")
	t.Cleanup(sub.Cancel)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	b.BridgeToHub(hub)
	b.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	if err := mw.Bus.PublishRunEvent(ctx, "run_bridge", 7); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-sub.Events:
		if ev.Index != 7 {
			t.Fatalf("Index=%d want 7", ev.Index)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hub subscriber got no PublishExternal nudge")
	}
}

func TestRedisLimiterFailOpen(t *testing.T) {
	mr := miniredis.RunT(t)
	mw, err := mwredis.Open(context.Background(), mwredis.Config{
		Addr: mr.Addr(), Stream: "baize:runs", ConsumerGroup: "baize-workers",
		EventsChannel: "baize:run-events", ConsumerName: "failopen-consumer",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mw.Close() })

	mr.Close()
	if !mw.Limiter.Allow("fail-open-key") {
		t.Fatal("Allow should fail-open (return true) when Redis is down")
	}
}

func TestRedisLimiterSharedBudgetAcrossInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	open := func(name string) *middleware.Middleware {
		t.Helper()
		mw, err := mwredis.Open(context.Background(), mwredis.Config{
			Addr: mr.Addr(), Stream: "baize:runs", ConsumerGroup: "baize-workers",
			EventsChannel: "baize:run-events", ConsumerName: name,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = mw.Close() })
		return mw
	}
	a := open("shared-a")
	b := open("shared-b")
	blA, ok := a.Limiter.(middleware.BudgetLimiter)
	if !ok {
		t.Fatal("limiter A does not implement BudgetLimiter")
	}
	blB, ok := b.Limiter.(middleware.BudgetLimiter)
	if !ok {
		t.Fatal("limiter B does not implement BudgetLimiter")
	}

	const limit = 5
	key := "baize:rl:inbox:shared-channel"
	allowed := 0
	for i := 0; i < limit; i++ {
		if blA.AllowBudget(key, limit, time.Minute) {
			allowed++
		}
	}
	for i := 0; i < limit; i++ {
		if blB.AllowBudget(key, limit, time.Minute) {
			allowed++
		}
	}
	if allowed != limit {
		t.Fatalf("shared budget allowed=%d want %d (two instances must share quota)", allowed, limit)
	}
	if blA.AllowBudget(key, limit, time.Minute) {
		t.Fatal("budget exhausted: further AllowBudget should be false")
	}
}

func TestRedisConsumeSurvivesIdleThenGetsJob(t *testing.T) {
	mr := miniredis.RunT(t)
	mw, err := mwredis.Open(context.Background(), mwredis.Config{
		Addr: mr.Addr(), Stream: "baize:runs", ConsumerGroup: "baize-workers",
		EventsChannel: "baize:run-events", ConsumerName: "idle-consumer",
		ConsumeBlock: 50 * time.Millisecond, // short so idle Nil happens quickly
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mw.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	type result struct {
		job middleware.Job
		ok  bool
	}
	ch := make(chan result, 1)
	go func() {
		got, ack, ok := mw.Queue.Consume(ctx)
		if ok && ack != nil {
			ack()
		}
		ch <- result{job: got, ok: ok}
	}()

	// Wait long enough for at least one empty XReadGroup BLOCK timeout.
	time.Sleep(150 * time.Millisecond)
	if err := mw.Queue.Enqueue(ctx, job("run_after_idle")); err != nil {
		t.Fatal(err)
	}

	select {
	case r := <-ch:
		if !r.ok || r.job.RunID != "run_after_idle" {
			t.Fatalf("after idle: ok=%v job=%+v", r.ok, r.job)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Consume did not return job after idle (worker loop likely exited on empty read)")
	}
}

func TestRedisConsumeCancelReturnsFalse(t *testing.T) {
	mr := miniredis.RunT(t)
	mw, err := mwredis.Open(context.Background(), mwredis.Config{
		Addr: mr.Addr(), Stream: "baize:runs", ConsumerGroup: "baize-workers",
		EventsChannel: "baize:run-events", ConsumerName: "cancel-consumer",
		// miniredis does not abort XReadGroup on ctx cancel mid-block; short
		// Block lets Consume notice cancel on the next loop iteration promptly.
		ConsumeBlock: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mw.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, _, ok := mw.Queue.Consume(ctx)
	elapsed := time.Since(start)
	if ok {
		t.Fatal("Consume should return ok=false after ctx cancel")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Consume took %v after cancel; want prompt shutdown", elapsed)
	}
}

func TestRedisConsumeSkipsPoisonKeepsGoing(t *testing.T) {
	mr := miniredis.RunT(t)
	const stream = "baize:runs"
	const group = "baize-workers"
	mw, err := mwredis.Open(context.Background(), mwredis.Config{
		Addr: mr.Addr(), Stream: stream, ConsumerGroup: group,
		EventsChannel: "baize:run-events", ConsumerName: "poison-consumer",
		ConsumeBlock: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mw.Close() })

	rc := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	if err := rc.XAdd(context.Background(), &goredis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"job": "{not-json"},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := mw.Queue.Enqueue(context.Background(), job("run_good")); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, ack, ok := mw.Queue.Consume(ctx)
	if !ok || got.RunID != "run_good" {
		t.Fatalf("expected good job after poison, ok=%v job=%+v", ok, got)
	}
	ack()
}

// TestNotifyAppendEventPublishesRunEventAcrossReplicas mirrors bootstrap wiring:
// replica A Notify+OnEvent→PublishRunEvent; replica B BridgeToHub receives
// PublishExternal after AppendEvent on A.
func TestNotifyAppendEventPublishesRunEventAcrossReplicas(t *testing.T) {
	mr := miniredis.RunT(t)
	open := func(name string) *middleware.Middleware {
		t.Helper()
		mw, err := mwredis.Open(context.Background(), mwredis.Config{
			Addr: mr.Addr(), Stream: "baize:runs-xrep", ConsumerGroup: "baize-workers",
			EventsChannel: "baize:run-events-xrep", ConsumerName: name,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = mw.Close() })
		return mw
	}
	a := open("replica-a")
	b := open("replica-b")

	hubA := eventbus.NewHub()
	hubB := eventbus.NewHub()
	st := eventbus.Notify(store.NewMemory(), hubA)

	// Same wiring as bootstrap: OnEvent → Bus.PublishRunEvent (skip nudge loop).
	hubA.OnEvent(func(runID string, ev eventbus.IndexedEvent) {
		if ev.Event.Type == "external.nudge" {
			return
		}
		if err := a.Bus.PublishRunEvent(context.Background(), runID, int64(ev.Index)); err != nil {
			t.Errorf("PublishRunEvent: %v", err)
		}
	})

	type hubBridge interface {
		BridgeToHub(hub *eventbus.Hub)
		Start(ctx context.Context)
	}
	bb, ok := b.Bus.(hubBridge)
	if !ok {
		t.Fatal("replica B bus missing BridgeToHub/Start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	bb.BridgeToHub(hubB)
	bb.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	sub := hubB.Subscribe("run_xrep")
	t.Cleanup(sub.Cancel)

	run, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	if err != nil {
		t.Fatal(err)
	}
	// Subscribe before Append using fixed ID so we know the key; recreate sub on run.ID.
	sub.Cancel()
	sub = hubB.Subscribe(run.ID)
	t.Cleanup(sub.Cancel)

	if err := st.AppendEvent(run.ID, store.Event{Type: "llm.message", Data: map[string]any{"content": "cross"}}); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-sub.Events:
		if ev.Event.Type != "external.nudge" {
			t.Fatalf("type=%q want external.nudge", ev.Event.Type)
		}
		if ev.Index != 0 {
			t.Fatalf("Index=%d want 0", ev.Index)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("replica B hub did not receive cross-replica PublishExternal")
	}
}
