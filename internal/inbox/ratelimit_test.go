package inbox_test

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/inbox"
)

func TestRateLimitBlocksBurst(t *testing.T) {
	rl := inbox.NewRateLimiter(2, time.Minute)
	if !rl.Allow("a") || !rl.Allow("a") {
		t.Fatal("first two should pass")
	}
	if rl.Allow("a") {
		t.Fatal("third should block")
	}
}

func TestRateLimitPerChannel(t *testing.T) {
	rl := inbox.NewRateLimiter(1, time.Minute)
	if !rl.Allow("a") {
		t.Fatal("first on a should pass")
	}
	if !rl.Allow("b") {
		t.Fatal("first on b should pass")
	}
	if rl.Allow("a") {
		t.Fatal("second on a should block")
	}
}
