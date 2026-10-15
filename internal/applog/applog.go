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
)

func Init(format string) { InitWithWriter(format, os.Stderr) }

func InitWithWriter(format string, w io.Writer) {
	f := strings.ToLower(strings.TrimSpace(format))
	var h slog.Handler
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	switch f {
	case "json":
		h = slog.NewJSONHandler(w, opts)
	default:
		h = slog.NewTextHandler(w, opts)
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
