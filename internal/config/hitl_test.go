package config

import (
	"testing"
	"time"
)

func TestHITLApprovalTTLResolver(t *testing.T) {
	var c Config
	if got := c.HITLApprovalTTL(); got != 24*time.Hour {
		t.Fatalf("unset ttl=%v want 24h", got)
	}

	zero := 0
	c.HITL.ApprovalTTLSec = &zero
	if got := c.HITLApprovalTTL(); got != 0 {
		t.Fatalf("explicit 0 ttl=%v want disabled", got)
	}

	sec := 90
	c.HITL.ApprovalTTLSec = &sec
	if got := c.HITLApprovalTTL(); got != 90*time.Second {
		t.Fatalf("ttl=%v want 90s", got)
	}

	negative := -5
	c.HITL.ApprovalTTLSec = &negative
	if got := c.HITLApprovalTTL(); got != 0 {
		t.Fatalf("negative ttl=%v want disabled", got)
	}
}
