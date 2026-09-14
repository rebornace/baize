package middleware

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/store"
)

// ReconcileStore is satisfied by store.Store directly.
type ReconcileStore interface {
	ListRunsForReconcile(limit int) ([]*store.Run, error)
}

// reconcileBatchLimit caps how many orphan runs a single reconcile pass
// re-enqueues; the next pass picks up the remainder.
const reconcileBatchLimit = 100

// Reconcile re-enqueues orphaned (queued/running with expired lease) runs.
// Re-enqueued jobs carry no UserParts: multimodal content is not persisted, so
// a reconciled image run replays with text input only (v0: such failures go
// through the normal run error path). Active runs are not duplicated: the
// worker-side LeaseRun acquisition fails for holders of a live lease.
func (mw *Middleware) Reconcile(ctx context.Context, rs ReconcileStore) {
	runs, err := rs.ListRunsForReconcile(reconcileBatchLimit)
	if err != nil {
		log.Printf("middleware: reconcile list: %v", err)
		return
	}
	for _, r := range runs {
		job := Job{
			RunID:      r.ID,
			Kind:       KindRun,
			AgentID:    r.AgentID,
			Input:      r.Input,
			EnqueuedAt: time.Now().UTC(),
		}
		// 用调和循环自己的 ctx：关停时不再入队，避免向已关闭/关停中的队列投递。
		if err := mw.Queue.Enqueue(ctx, job); err != nil {
			log.Printf("middleware: reconcile enqueue %s: %v", r.ID, err)
		}
	}
}

// StartReconciler runs Reconcile immediately and every interval until ctx
// ends. It returns a stop func that cancels the loop and waits for the
// goroutine to exit. A non-positive interval defaults to 15s.
func (mw *Middleware) StartReconciler(ctx context.Context, rs ReconcileStore, interval time.Duration) func() {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		mw.Reconcile(runCtx, rs)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-t.C:
				mw.Reconcile(runCtx, rs)
			}
		}
	}()
	return func() { cancel(); wg.Wait() }
}
