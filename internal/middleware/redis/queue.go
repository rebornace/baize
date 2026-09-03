package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/middleware"
	goredis "github.com/redis/go-redis/v9"
)

const defaultConsumeBlock = 5 * time.Second

// queue implements middleware.JobQueue on Redis Streams + consumer groups.
//
// XAUTOCLAIM is intentionally not implemented in v0. Crash recovery relies on
// the DB reconciler re-claiming expired leases and re-XADDing the run; the
// consumer-group PEL may accumulate stale entries after worker crashes.
// Operators may XTRIM the stream or periodically clean the PEL. A future
// revision may add periodic XAUTOCLAIM as a secondary safety net.
type queue struct {
	client   *goredis.Client
	stream   string
	group    string
	consumer string
	block    time.Duration
}

func newQueue(c *goredis.Client, stream, group, consumer string, block time.Duration) *queue {
	if block <= 0 {
		block = defaultConsumeBlock
	}
	return &queue{client: c, stream: stream, group: group, consumer: consumer, block: block}
}

func (q *queue) ensureGroup(ctx context.Context) error {
	err := q.client.XGroupCreateMkStream(ctx, q.stream, q.group, "$").Err()
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return fmt.Errorf("redis queue: create group: %w", err)
}

// Enqueue serializes job and XADDs it. Redis ops use a detached short-timeout
// context so a cancelled request ctx cannot turn a successful XADD into an
// Enqueue error (which would trigger Dispatch local fallback + worker double-run).
func (q *queue) Enqueue(ctx context.Context, job middleware.Job) error {
	if job.EnqueuedAt.IsZero() {
		job.EnqueuedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("redis queue: marshal job: %w", err)
	}
	opCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return q.client.XAdd(opCtx, &goredis.XAddArgs{
		Stream: q.stream,
		Values: map[string]any{"job": string(payload)},
	}).Err()
}

// Consume blocks until a valid job is available or ctx ends.
// ok=false only means shutdown (ctx cancelled/deadline) — matching the memory
// queue contract so workers that treat !ok as permanent exit stay alive across
// idle XReadGroup timeouts, recoverable Redis errors, and poison messages.
func (q *queue) Consume(ctx context.Context) (middleware.Job, func(), bool) {
	for {
		if ctx.Err() != nil {
			return middleware.Job{}, nil, false
		}
		res, err := q.client.XReadGroup(ctx, &goredis.XReadGroupArgs{
			Group:    q.group,
			Consumer: q.consumer,
			Streams:  []string{q.stream, ">"},
			Count:    1,
			Block:    q.block,
		}).Result()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				return middleware.Job{}, nil, false
			}
			if errors.Is(err, goredis.Nil) {
				// Idle block timeout: keep waiting (do not signal worker exit).
				continue
			}
			// Recoverable Redis/network error: brief backoff, still watch ctx.
			select {
			case <-ctx.Done():
				return middleware.Job{}, nil, false
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}
		if len(res) == 0 || len(res[0].Messages) == 0 {
			continue
		}
		msg := res[0].Messages[0]
		raw, _ := msg.Values["job"].(string)
		var job middleware.Job
		if err := json.Unmarshal([]byte(raw), &job); err != nil {
			// Poison message: ack to avoid infinite redelivery, then keep consuming.
			q.makeAck(msg.ID)()
			continue
		}
		return job, q.makeAck(msg.ID), true
	}
}

// makeAck returns a panic-safe XACK closure so network/client panics cannot
// take down the worker process (task 5 ledger).
func (q *queue) makeAck(msgID string) func() {
	return func() {
		defer func() { _ = recover() }()
		ackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = q.client.XAck(ackCtx, q.stream, q.group, msgID).Err()
	}
}

func (q *queue) Close() error { return nil }
