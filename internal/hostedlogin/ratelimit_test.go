package hostedlogin

import (
	"testing"
	"time"
)

func TestRateLimiterAllowHosted(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	if !rl.AllowHosted("1.1.1.1", "s1") {
		t.Fatal("first")
	}
	if !rl.AllowHosted("1.1.1.1", "s1") {
		t.Fatal("second")
	}
	if rl.AllowHosted("1.1.1.1", "s2") {
		t.Fatal("ip budget exhausted")
	}
	if !rl.AllowHosted("2.2.2.2", "s3") {
		t.Fatal("other ip ok")
	}
}
