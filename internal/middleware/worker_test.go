package middleware_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/middleware"
	_ "github.com/rebornace/baize/internal/middleware/memory"
)

type fakeExecutor struct {
	mu      sync.Mutex
	ran     []string
	panicOn map[string]bool
}

func (f *fakeExecutor) ExecuteJob(_ context.Context, j middleware.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.panicOn != nil && f.panicOn[j.RunID] {
		panic("boom")
	}
	f.ran = append(f.ran, j.RunID)
	return nil
}

func (f *fakeExecutor) ranCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.ran)
}

func waitForRan(t *testing.T, ex *fakeExecutor, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ex.ranCount() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("worker did not process %d jobs (got %d)", want, ex.ranCount())
}

func TestWorkerProcessesEnqueuedJob(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{WorkerConcurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()
	ex := &fakeExecutor{}
	stop := mw.StartWorkers(context.Background(), ex)
	defer stop()

	if err := mw.Queue.Enqueue(context.Background(), middleware.Job{RunID: "run_w1", Kind: middleware.KindRun, Input: "x"}); err != nil {
		t.Fatal(err)
	}
	waitForRan(t, ex, 1)
}

func TestStartWorkersNilSafe(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()
	// 不传 Executor 时不应 panic：worker 仅 drain-and-ack（测试用）。
	stop := mw.StartWorkers(context.Background(), nil)
	if err := mw.Queue.Enqueue(context.Background(), middleware.Job{RunID: "run_drain", Kind: middleware.KindRun}); err != nil {
		t.Fatal(err)
	}
	stop()
}

func TestWorkerRecoversExecutorPanic(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{WorkerConcurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()
	ex := &fakeExecutor{panicOn: map[string]bool{"run_panic": true}}
	stop := mw.StartWorkers(context.Background(), ex)
	defer stop()

	if err := mw.Queue.Enqueue(context.Background(), middleware.Job{RunID: "run_panic", Kind: middleware.KindRun}); err != nil {
		t.Fatal(err)
	}
	if err := mw.Queue.Enqueue(context.Background(), middleware.Job{RunID: "run_after", Kind: middleware.KindRun}); err != nil {
		t.Fatal(err)
	}
	// panic 的 job 不入 ran；后续 job 仍被同一 worker 处理，证明 goroutine 未崩溃。
	waitForRan(t, ex, 1)
	ex.mu.Lock()
	got := ex.ran[0]
	ex.mu.Unlock()
	if got != "run_after" {
		t.Fatalf("expected run_after after panic, got %q", got)
	}
}

func TestStartWorkersStopWaitsForInFlight(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{WorkerConcurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()

	started := make(chan struct{})
	release := make(chan struct{})
	var done int32
	var mu sync.Mutex
	ex := executorFunc(func(_ context.Context, _ middleware.Job) error {
		close(started)
		<-release
		mu.Lock()
		done = 1
		mu.Unlock()
		return nil
	})

	stop := mw.StartWorkers(context.Background(), ex)
	if err := mw.Queue.Enqueue(context.Background(), middleware.Job{RunID: "run_inflight", Kind: middleware.KindRun}); err != nil {
		t.Fatal(err)
	}
	<-started
	// stop 必须等待在飞 job 结束：先触发 stop（阻塞等待），再放行 job。
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	close(release)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not return after in-flight job finished")
	}
	mu.Lock()
	defer mu.Unlock()
	if done != 1 {
		t.Fatal("job did not finish before stop returned")
	}
}

type executorFunc func(ctx context.Context, job middleware.Job) error

func (f executorFunc) ExecuteJob(ctx context.Context, job middleware.Job) error { return f(ctx, job) }

// TestWorkerJobCtxNotCanceledOnStop 断言关停语义：作业执行 ctx 脱离 worker
// 的消费/关停生命周期——stop() 返回后，在飞作业持有的 ctx 仍未被取消
// （否则 HITL 等待中的 run 会被引擎 markCancelled，重开后无法 cold resume）。
func TestWorkerJobCtxNotCanceledOnStop(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{WorkerConcurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()

	started := make(chan struct{})
	release := make(chan struct{})
	jobCtx := make(chan context.Context, 1)
	ex := executorFunc(func(ctx context.Context, _ middleware.Job) error {
		jobCtx <- ctx
		close(started)
		<-release
		return nil
	})

	stop := mw.StartWorkers(context.Background(), ex)
	if err := mw.Queue.Enqueue(context.Background(), middleware.Job{RunID: "run_ctx", Kind: middleware.KindRun}); err != nil {
		t.Fatal(err)
	}
	<-started

	// stop 在宽限内等待作业；先放行让其结束，stop 正常返回。
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	close(release)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not return after job finished")
	}

	// 作业已随 stop 前完成：它持有的 ctx 自始至终不应被取消。
	ctx := <-jobCtx
	if err := ctx.Err(); err != nil {
		t.Fatalf("job ctx was canceled on shutdown: %v", err)
	}
}
