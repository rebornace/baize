package middleware

import (
	"context"
	"log"
	"sync"
)

// Executor processes a single job (implemented by api.Server).
type Executor interface {
	ExecuteJob(ctx context.Context, job Job) error
}

const defaultWorkerConcurrency = 8

// StartWorkers runs mw.concurrency competing consumers over mw.Queue. It
// returns a stop func that cancels the run context and waits for in-flight
// jobs to finish. A nil Executor makes workers drain-and-ack (tests only).
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
				mw.runJob(runCtx, ex, job, ack)
			}
		}()
	}

	return func() {
		cancel()
		wg.Wait()
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
