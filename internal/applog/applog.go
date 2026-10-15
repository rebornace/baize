package applog

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

var (
	mu     sync.RWMutex
	logger = slog.Default()
	ring   = newRing(defaultRingCap)
)

func Init(format string) { InitWithWriter(format, os.Stderr) }

func InitWithWriter(format string, w io.Writer) {
	f := strings.ToLower(strings.TrimSpace(format))
	var h slog.Handler
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	tee := io.MultiWriter(w, ring)
	switch f {
	case "json":
		h = slog.NewJSONHandler(tee, opts)
	default:
		h = slog.NewTextHandler(tee, opts)
	}
	l := slog.New(h)
	mu.Lock()
	logger = l
	mu.Unlock()
	slog.SetDefault(l)
}

func L() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return logger
}

// Recent returns up to limit of the most recent log lines (oldest first).
// limit <= 0 means return all buffered lines.
func Recent(limit int) []string {
	return ring.Recent(limit)
}

// ResetRingForTest clears the in-memory log buffer (tests only).
func ResetRingForTest() {
	ring.Reset()
}
