package hostedlogin

import (
	"context"
	"sync"
	"time"
)

// DefaultAwaitTimeout caps how long an MCP tools/call waits for hosted login.
const DefaultAwaitTimeout = 10 * time.Minute

// Waiter lets tools/call hang until Submit (or OAuth callback) finishes for a session.
type Waiter interface {
	Await(ctx context.Context, sessionID string) error
	NotifyOK(sessionID string)
	NotifyFail(sessionID string, err error)
}

type memoryWaiter struct {
	mu      sync.Mutex
	pending map[string]chan error
}

// NewMemoryWaiter returns an in-process login completion hub.
func NewMemoryWaiter() Waiter {
	return &memoryWaiter{pending: make(map[string]chan error)}
}

func (w *memoryWaiter) Await(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return context.Canceled
	}
	ch := make(chan error, 1)
	w.mu.Lock()
	if old, ok := w.pending[sessionID]; ok {
		close(old)
	}
	w.pending[sessionID] = ch
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		if w.pending[sessionID] == ch {
			delete(w.pending, sessionID)
		}
		w.mu.Unlock()
	}()

	timeout := DefaultAwaitTimeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err, ok := <-ch:
		if !ok {
			return context.Canceled
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return context.DeadlineExceeded
	}
}

func (w *memoryWaiter) NotifyOK(sessionID string) {
	w.notify(sessionID, nil)
}

func (w *memoryWaiter) NotifyFail(sessionID string, err error) {
	w.notify(sessionID, err)
}

func (w *memoryWaiter) notify(sessionID string, err error) {
	if sessionID == "" {
		return
	}
	w.mu.Lock()
	ch, ok := w.pending[sessionID]
	if ok {
		delete(w.pending, sessionID)
	}
	w.mu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- err:
	default:
	}
}
