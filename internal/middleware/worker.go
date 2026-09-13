package middleware

import (
	"context"
	"log"
	"sync"
	"time"
)

// Executor processes a single job (implemented by api.Server).
type Executor interface {
	ExecuteJob(ctx context.Context, job Job) error
}

const defaultWorkerConcurrency = 8

// workerShutdownGrace bounds how long the stop func waits for in-flight jobs
// before returning. Ordinary jobs finish in milliseconds and are awaited
// within the grace; a long-blocking job (e.g. a run parked in HITL waiting for
// human input) outlives the grace and is left running as a detached goroutine,
// mirroring the legacy fire-and-forget semantics — the DB remains authoritative
// and the reconciler / cold resume on restart picks the run up. It is a
// package-level variable (not const) so tests can shrink it.
var workerShutdownGrace = 5 * time.Second

// StartWorkers runs mw.concurrency competing consumers over mw.Queue. It
// returns a stop func that cancels the consume context (no new jobs are
// pulled) and waits up to workerShutdownGrace for in-flight jobs to finish;
// jobs still blocked when the grace expires are detached and stop returns
// anyway.
//
// Jobs execute under a per-job context.Background(), deliberately NOT derived
// from the consume context: shutting the server down must not cancel runs that
// are in flight (in particular a run parked in HITL must stay waiting_human in
// the DB so a cold resume after reopen works). Cooperative run cancellation is
// handled separately by the engine's own Cancel(runID) registry. The consume
// context only governs pulling new jobs (Consume returns once it is done).
// A nil Executor makes workers drain-and-ack (tests only).
func (mw *Middleware) StartWorkers(ctx context.Context, ex Executor) func() {
	concurrency := mw.concurrency
	if concurrency <= 0 {
		concurrency = defaultWorkerConcurrency
	}

	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				job, ack, ok := mw.Queue.Consume(runCtx)
				if !ok {
					return
				}
				// 作业 ctx 脱离关停生命周期：每个作业一个独立 Background，
				// 关停只停止拉取新作业，不取消在飞 run。
				mw.runJob(context.Background(), ex, job, ack)
			}
		}()
	}

	return func() {
		cancel()
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			// 所有 worker（含在飞作业）在宽限内退出。
		case <-time.After(workerShutdownGrace):
			// 仍有 worker 卡在长作业（如 HITL 等人）：不再等待，这些作业
			// 作为脱离的 goroutine 继续运行（或随进程结束）；DB 权威状态
			// 与重启后的 reconciler/冷恢复兜底，沿用旧 fire-and-forget 语义。
		}
	}
}

func (mw *Middleware) runJob(ctx context.Context, ex Executor, job Job, ack func()) {
	// ack 必须在任何路径（成功/失败/panic）下都被调用，避免消息悬挂。
	defer func() {
		if r := recover(); r != nil {
			log.Printf("middleware: worker recovered from job %s panic: %v", job.RunID, r)
		}
		if ack != nil {
			ack()
		}
	}()

	if ex == nil {
		return
	}
	if err := ex.ExecuteJob(ctx, job); err != nil {
		log.Printf("middleware: job %s failed: %v", job.RunID, err)
	}
}
