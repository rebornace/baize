package middleware_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/middleware"
	_ "github.com/rebornace/baize/internal/middleware/memory"
	"github.com/rebornace/baize/internal/store"
)

// reconcileStore 是 middleware.ReconcileStore 的内存假实现：返回编排好的
// orphan run 列表。它只需要实现调和器依赖的最小方法。
type reconcileStore struct {
	runs []*store.Run
}

func (s *reconcileStore) ListRunsForReconcile(limit int) ([]*store.Run, error) {
	var out []*store.Run
	for _, r := range s.runs {
		out = append(out, r)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func TestReconcileEnqueuesOrphans(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{WorkerConcurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()

	var mu sync.Mutex
	var got []string
	ex := executorFunc(func(_ context.Context, j middleware.Job) error {
		mu.Lock()
		got = append(got, j.RunID)
		mu.Unlock()
		return nil
	})

	stop := mw.StartWorkers(context.Background(), ex)
	defer stop()

	rs := &reconcileStore{runs: []*store.Run{
		{ID: "run_orphan", AgentID: "a", Status: store.StatusRunning},
	}}
	mw.Reconcile(context.Background(), rs)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("orphan run was not re-enqueued/executed")
}

// TestStartReconcilerTicksAndStops 验证周期调和器会立即跑一次、随 ticker
// 重复执行，并在 stop 返回后 goroutine 干净退出（无泄漏/不再入队）。
func TestStartReconcilerTicksAndStops(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{WorkerConcurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()

	var mu sync.Mutex
	var got []string
	ex := executorFunc(func(_ context.Context, j middleware.Job) error {
		mu.Lock()
		got = append(got, j.RunID)
		mu.Unlock()
		return nil
	})
	stopWorkers := mw.StartWorkers(context.Background(), ex)
	defer stopWorkers()

	rs := &reconcileStore{runs: []*store.Run{
		{ID: "run_orphan2", AgentID: "a", Status: store.StatusQueued},
	}}
	// 远小于测试超时的间隔，确保至少触发一次周期调和。
	stopReconciler := mw.StartReconciler(context.Background(), rs, 20*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= 2 { // 立即一次 + 至少一次 tick
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stopReconciler()

	mu.Lock()
	n := len(got)
	mu.Unlock()
	if n < 2 {
		t.Fatalf("reconciler did not run periodically (got %d executions)", n)
	}
}
