package middleware_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/middleware"
	_ "github.com/rebornace/baize/internal/middleware/memory"
)

func TestOpenUnknownDriverErrors(t *testing.T) {
	if _, err := middleware.Open(context.Background(), "nope", middleware.Options{}); err == nil {
		t.Fatal("unknown driver must error")
	}
}

func TestRegisterAndOpen(t *testing.T) {
	mw, err := middleware.Open(context.Background(), "memory", middleware.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if mw.Queue == nil || mw.Bus == nil || mw.Limiter == nil {
		t.Fatalf("memory middleware must wire all: %+v", mw)
	}
}

func TestListDriversIncludesMemory(t *testing.T) {
	found := false
	for _, d := range middleware.ListDrivers() {
		if d == "memory" {
			found = true
		}
	}
	if !found {
		t.Fatalf("memory must be registered: %v", middleware.ListDrivers())
	}
}
