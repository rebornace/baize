// Package convlock serializes work per conversation inside one process.
//
// It exists because checking "does this conversation already have an active
// Run?" and creating the Run must happen in the same critical section; doing
// the check and the create in two separate steps lets two callers pass the
// check together and start parallel Runs on the same conversation.
//
// The lock is process-local by design. Multi-replica deployments still need a
// database-level guard; that is documented as a known limit.
package convlock

import "sync"

type entry struct {
	mu   sync.Mutex
	refs int
}

// Guard is a keyed mutex with reference-counted cleanup.
type Guard struct {
	mu      sync.Mutex
	entries map[string]*entry
}

// New returns an empty Guard.
func New() *Guard {
	return &Guard{entries: map[string]*entry{}}
}

// Do runs fn while holding the lock for key. Calls with the same key are
// serialized; different keys run concurrently. An empty key bypasses the lock
// (machine paths without a conversation have nothing to serialize against).
func (g *Guard) Do(key string, fn func() error) error {
	if g == nil || key == "" {
		return fn()
	}

	g.mu.Lock()
	e := g.entries[key]
	if e == nil {
		e = &entry{}
		g.entries[key] = e
	}
	e.refs++
	g.mu.Unlock()

	e.mu.Lock()
	err := fn()
	e.mu.Unlock()

	g.mu.Lock()
	e.refs--
	if e.refs == 0 {
		delete(g.entries, key)
	}
	g.mu.Unlock()
	return err
}

// size reports the number of tracked keys (tests only).
func (g *Guard) size() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.entries)
}
