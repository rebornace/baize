package identity

import (
	"context"
	"testing"
)

func TestRunAndAgentIDContext(t *testing.T) {
	ctx := context.Background()
	if RunIDFrom(ctx) != "" || AgentIDFrom(ctx) != "" {
		t.Fatalf("empty ctx should yield empty ids")
	}
	ctx = WithRunID(ctx, "run_1")
	ctx = WithAgentID(ctx, "agent_a")
	if got := RunIDFrom(ctx); got != "run_1" {
		t.Fatalf("RunIDFrom=%q", got)
	}
	if got := AgentIDFrom(ctx); got != "agent_a" {
		t.Fatalf("AgentIDFrom=%q", got)
	}
}
