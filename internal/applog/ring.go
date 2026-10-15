package applog

import (
	"strings"
	"sync"
)

const defaultRingCap = 500

// ringBuffer keeps recent log lines for the admin UI (tee'd from slog output).
type ringBuffer struct {
	mu    sync.Mutex
	lines []string
	cap   int
}

func newRing(capacity int) *ringBuffer {
	if capacity < 1 {
		capacity = defaultRingCap
	}
	return &ringBuffer{cap: capacity, lines: make([]string, 0, capacity)}
}

func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, line := range strings.Split(string(p), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if len(r.lines) >= r.cap {
			copy(r.lines, r.lines[1:])
			r.lines[len(r.lines)-1] = line
		} else {
			r.lines = append(r.lines, line)
		}
	}
	return len(p), nil
}

func (r *ringBuffer) Recent(limit int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(r.lines)
	if n == 0 {
		return nil
	}
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]string, limit)
	copy(out, r.lines[n-limit:])
	return out
}

func (r *ringBuffer) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = r.lines[:0]
}
