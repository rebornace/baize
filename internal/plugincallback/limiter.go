package plugincallback

import (
	"sync"
	"time"
)

// Limiter is an in-memory per-runID rate limiter using a fixed sliding window
// of one hour. The default budget is 100 callbacks per run per hour, matching
// the v0 design spec. A Limiter is safe for concurrent use.
type Limiter struct {
	mu      sync.Mutex
	budget  int
	window  time.Duration
	buckets map[string]*bucket
}

type bucket struct {
	count    int
	windowID int64
}

// NewLimiter returns a Limiter with the given per-run budget per window.
// Use DefaultBudget / DefaultWindow for the spec defaults.
func NewLimiter(budget int, window time.Duration) *Limiter {
	if budget <= 0 {
		budget = DefaultBudget
	}
	if window <= 0 {
		window = DefaultWindow
	}
	return &Limiter{
		budget:  budget,
		window:  window,
		buckets: make(map[string]*bucket),
	}
}

// Allow reports whether runID may perform one more callback at now. It
// increments the run's counter when allowed and rolls the window when now
// crosses into a new window.
func (l *Limiter) Allow(runID string, now time.Time) bool {
	if runID == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	wid := now.Unix() / int64(l.window/time.Second)
	b, ok := l.buckets[runID]
	if !ok || b.windowID != wid {
		b = &bucket{windowID: wid}
		l.buckets[runID] = b
	}
	if b.count >= l.budget {
		return false
	}
	b.count++
	return true
}

// Reset clears all counters. Useful for tests.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buckets = make(map[string]*bucket)
}
