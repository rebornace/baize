package middleware

import (
	"context"
	"testing"
	"time"
)

// stubQueue 是包内测试用的最小 JobQueue：带缓冲 channel 投递，Consume 在
// ctx 结束时返回 ok=false。不依赖 memory 驱动（包内测试不能 import memory，
// 否则 import 循环）。
type stubQueue struct {
	ch chan Job
}

func newStubQueue() *stubQueue {
	return &stubQueue{ch: make(chan Job, 16)}
}

func (q *stubQueue) Enqueue(_ context.Context, j Job) error {
	q.ch <- j
	return nil
}

func (q *stubQueue) Consume(ctx context.Context) (Job, func(), bool) {
	select {
	case j := <-q.ch:
		return j, func() {}, true
	case <-ctx.Done():
		return Job{}, nil, false
	}
}

func (q *stubQueue) Close() error { return nil }

type blockingExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (e *blockingExecutor) ExecuteJob(context.Context, Job) error {
	close(e.started)
	<-e.release // 模拟 HITL 长等：直到测试结束才放行
	return nil
}

// TestStartWorkersStopReturnsWithinGraceWhenJobBlocks 断言关停宽限语义：
// 执行器永久阻塞时 stop() 不被无限挂死，而是在 workerShutdownGrace 量级内
// 返回（残留作业作为脱离 goroutine 留下）。包内测试把 grace 缩到 100ms，
// 保证测试快且不 flake。
func TestStartWorkersStopReturnsWithinGraceWhenJobBlocks(t *testing.T) {
	oldGrace := workerShutdownGrace
	workerShutdownGrace = 100 * time.Millisecond
	t.Cleanup(func() { workerShutdownGrace = oldGrace })

	mw := &Middleware{Queue: newStubQueue(), concurrency: 1}
	ex := &blockingExecutor{started: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() { close(ex.release) }) // 测试结束放行脱离的 goroutine

	stop := mw.StartWorkers(context.Background(), ex)
	if err := mw.Queue.Enqueue(context.Background(), Job{RunID: "run_block", Kind: KindRun}); err != nil {
		t.Fatal(err)
	}
	<-ex.started

	done := make(chan struct{})
	start := time.Now()
	go func() { stop(); close(done) }()
	select {
	case <-done:
		// 应在 ~grace（100ms）处返回；给足调度余量，上界 1s。
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("stop took %v, expected ~grace (%v)", elapsed, workerShutdownGrace)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("stop hung past grace (%v) on a blocked job", workerShutdownGrace)
	}
}

// TestStartWorkersStopAwaitsQuickJobsWithinGrace 是宽限的正向面：毫秒级
// 完成的普通作业必须在 stop 返回前被等到（不因为宽限机制而提前放弃）。
func TestStartWorkersStopAwaitsQuickJobsWithinGrace(t *testing.T) {
	oldGrace := workerShutdownGrace
	workerShutdownGrace = 100 * time.Millisecond
	t.Cleanup(func() { workerShutdownGrace = oldGrace })

	mw := &Middleware{Queue: newStubQueue(), concurrency: 1}
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	ex := executorFuncInternal(func(context.Context, Job) error {
		close(started)
		<-release
		close(finished)
		return nil
	})

	stop := mw.StartWorkers(context.Background(), ex)
	if err := mw.Queue.Enqueue(context.Background(), Job{RunID: "run_quick", Kind: KindRun}); err != nil {
		t.Fatal(err)
	}
	<-started

	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	// stop 先进入等待；放行作业，它应在宽限内完成并让 stop 返回。
	close(release)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not return after in-flight job finished within grace")
	}
	select {
	case <-finished:
	default:
		t.Fatal("job did not finish before stop returned")
	}
}

type executorFuncInternal func(context.Context, Job) error

func (f executorFuncInternal) ExecuteJob(ctx context.Context, j Job) error { return f(ctx, j) }
