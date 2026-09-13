package middleware

import (
	"context"
	"time"
)

// JobQueue delivers run jobs to competing consumers.
type JobQueue interface {
	// Enqueue posts a job for any worker to process.
	Enqueue(ctx context.Context, job Job) error
	// Consume blocks until a job is available or ctx is cancelled. On success
	// it returns the job and an ack func the worker calls after processing
	// (ack nil-safe: implementations must accept a nil no-op gracefully).
	Consume(ctx context.Context) (job Job, ack func(), ok bool)
	Close() error
}

// EventBus fans a lightweight "run X has new events up to seq" signal to all
// replicas. Event content is always re-read from the DB; this is a nudge only.
type EventBus interface {
	PublishRunEvent(ctx context.Context, runID string, seq int64) error
	SubscribeRunEvents(ctx context.Context) (<-chan RunEventNudge, error)
	Close() error
}

// RunEventNudge is a cross-replica wake-up signal.
type RunEventNudge struct {
	RunID string
	Seq   int64
}

// Limiter is a distributed-or-local rate limiter (Allow-style).
type Limiter interface {
	Allow(key string) bool
}

// BudgetLimiter is implemented by limiters that support an explicit quota/window.
type BudgetLimiter interface {
	AllowBudget(key string, limit int, window time.Duration) bool
}

// Options configures a driver. Redis fields are ignored by the memory driver.
type Options struct {
	WorkerConcurrency int
	LeaseTTL          time.Duration
	ReconcileInterval time.Duration
	Redis             RedisOptions
}

// RedisOptions are connection/stream settings for the redis driver.
type RedisOptions struct {
	Addr          string
	DB            int
	Username      string
	Password      string
	Stream        string
	ConsumerGroup string
	EventsChannel string
}

// Middleware is the wired bundle handed to bootstrap.
type Middleware struct {
	Queue   JobQueue
	Bus     EventBus
	Limiter Limiter
	// Close shuts the driver down (queue, bus, pools).
	Close func() error

	concurrency int // 由 Open 从 Options.WorkerConcurrency 设置，驱动工厂无需关心
}

// Open builds a Middleware for the named driver.
func Open(ctx context.Context, driver string, opts Options) (*Middleware, error) {
	f, err := lookupDriver(driver)
	if err != nil {
		return nil, err
	}
	mw, err := f(ctx, opts)
	if err != nil {
		return nil, err
	}
	mw.concurrency = opts.WorkerConcurrency
	return mw, nil
}
