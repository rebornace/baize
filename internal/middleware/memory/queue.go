package memory

import (
	"context"
	"errors"
	"sync"

	"github.com/rebornace/baize/internal/middleware"
)

type queue struct {
	ch   chan middleware.Job
	done chan struct{}
	once sync.Once
}

func newQueue() *queue {
	return &queue{ch: make(chan middleware.Job, 1024), done: make(chan struct{})}
}

var errQueueClosed = errors.New("middleware: job queue closed")

func (q *queue) Enqueue(ctx context.Context, job middleware.Job) error {
	if job.EnqueuedAt.IsZero() {
		job.EnqueuedAt = nowUTC()
	}
	// 关闭优先：保证 Close 返回后的 Enqueue 确定性报错，而非与发送分支随机竞争。
	select {
	case <-q.done:
		return errQueueClosed
	default:
	}
	select {
	case q.ch <- job:
		return nil
	case <-q.done:
		return errQueueClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *queue) Consume(ctx context.Context) (middleware.Job, func(), bool) {
	select {
	case job, ok := <-q.ch:
		if !ok {
			return middleware.Job{}, nil, false
		}
		return job, func() {}, true
	case <-q.done:
		return middleware.Job{}, nil, false
	case <-ctx.Done():
		return middleware.Job{}, nil, false
	}
}

// Close 幂等：只关闭 done 信号，不 close 数据 channel（彻底避免 send-on-closed）。
func (q *queue) Close() error {
	q.once.Do(func() { close(q.done) })
	return nil
}
