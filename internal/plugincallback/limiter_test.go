package plugincallback

import (
	"testing"
	"time"
)

func TestLimiterAllowsUnderBudget(t *testing.T) {
	l := NewLimiter(3, time.Hour)
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < 3; i++ {
		if !l.Allow("run_x", now) {
			t.Fatalf("call %d rejected", i)
		}
	}
	if l.Allow("run_x", now) {
		t.Fatal("4th call should be rejected")
	}
}

func TestLimiterPerRunIsolated(t *testing.T) {
	l := NewLimiter(2, time.Hour)
	now := time.Unix(1_700_000_000, 0)
	if !l.Allow("run_a", now) || !l.Allow("run_a", now) {
		t.Fatal("run_a calls rejected")
	}
	if l.Allow("run_a", now) {
		t.Fatal("run_a 3rd should reject")
	}
	if !l.Allow("run_b", now) || !l.Allow("run_b", now) {
		t.Fatal("run_b calls rejected")
	}
}

func TestLimiterWindowReset(t *testing.T) {
	l := NewLimiter(2, time.Hour)
	t0 := time.Unix(1_700_000_000, 0)
	if !l.Allow("run_r", t0) || !l.Allow("run_r", t0) {
		t.Fatal("first window calls rejected")
	}
	if l.Allow("run_r", t0) {
		t.Fatal("over-budget in first window should reject")
	}
	// Advance into the next window (one hour later).
	t1 := t0.Add(time.Hour + time.Second)
	if !l.Allow("run_r", t1) {
		t.Fatal("new window should allow")
	}
}

func TestLimiterEmptyRunIDRejected(t *testing.T) {
	l := NewLimiter(10, time.Hour)
	if l.Allow("", time.Unix(1_700_000_000, 0)) {
		t.Fatal("empty runID should be rejected")
	}
}

func TestNewLimiterAppliesDefaults(t *testing.T) {
	l := NewLimiter(0, 0)
	if l.budget != DefaultBudget {
		t.Fatalf("budget=%d want %d", l.budget, DefaultBudget)
	}
	if l.window != DefaultWindow {
		t.Fatalf("window=%v want %v", l.window, DefaultWindow)
	}
}
