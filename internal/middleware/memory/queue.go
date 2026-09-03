package memory

import (
	"context"

	"github.com/rebornace/baize/internal/middleware"
)

type queue struct {
	ch chan middleware.Job
}

func newQueue() *queue {
	return &queue{ch: make(chan middleware.Job, 1024)}
}

func (q *queue) Enqueue(_ context.Context, job middleware.Job) error {
	if job.EnqueuedAt.IsZero() {
		job.EnqueuedAt = nowUTC()
	}
	select {
	case q.ch <- job:
		return nil
	default:
		// 缓冲满：阻塞投递由调用方 ctx 控制，避免无界 goroutine。
		q.ch <- job
		return nil
	}
}

func (q *queue) Consume(ctx context.Context) (middleware.Job, func(), bool) {
	select {
	case job := <-q.ch:
		return job, func() {}, true
	case <-ctx.Done():
		return middleware.Job{}, nil, false
	}
}

func (q *queue) Close() error { close(q.ch); return nil }
