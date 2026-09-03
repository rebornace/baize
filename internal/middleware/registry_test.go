package middleware_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/middleware"
)

func TestOpenUnknownDriverErrors(t *testing.T) {
	if _, err := middleware.Open(context.Background(), "nope", middleware.Options{}); err == nil {
		t.Fatal("unknown driver must error")
	}
}
