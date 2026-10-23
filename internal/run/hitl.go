package run

import (
	"context"
	"fmt"
	"sync"

	"github.com/rebornace/baize/internal/controlplane"
)

// Decision is a human approval or rejection for a waiting run.
// Role and OperatorID identify the approver so tools that check control-plane
// ACL (e.g. propose_skill) see the resume principal after Gate.Resume, which
// otherwise drops the HTTP request context.
type Decision struct {
	Approve    bool
	Comment    string
	Role       controlplane.Role
	OperatorID string
}

// applyDecisionPrincipal copies the approver identity from d onto ctx when set.
func applyDecisionPrincipal(ctx context.Context, d Decision) context.Context {
	if d.Role != controlplane.RoleNone {
		ctx = controlplane.WithRole(ctx, d.Role)
	}
	if d.OperatorID != "" {
		ctx = controlplane.WithOperatorID(ctx, d.OperatorID)
	}
	return ctx
}

// Gate coordinates same-process HITL waits via per-run channels.
// Cross-restart resume does not rely on Gate; use Engine.ContinueFromHITL.
type Gate struct {
	mu      sync.Mutex
	waiters map[string]chan Decision
}

// NewGate creates an empty HITL gate.
func NewGate() *Gate {
	return &Gate{waiters: make(map[string]chan Decision)}
}

// BeginWait registers a waiter channel for runID. Caller must EndWait.
// Register before advertising waiting_human so Resume cannot miss the channel.
func (g *Gate) BeginWait(runID string) (<-chan Decision, error) {
	if g == nil {
		return nil, fmt.Errorf("hitl: nil gate")
	}
	ch := make(chan Decision, 1)
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.waiters[runID]; exists {
		return nil, fmt.Errorf("hitl: already waiting for %s", runID)
	}
	g.waiters[runID] = ch
	return ch, nil
}

// EndWait removes the waiter for runID.
func (g *Gate) EndWait(runID string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	delete(g.waiters, runID)
	g.mu.Unlock()
}

// Wait blocks until Resume delivers a decision for runID, or ctx is done.
func (g *Gate) Wait(ctx context.Context, runID string) (Decision, error) {
	ch, err := g.BeginWait(runID)
	if err != nil {
		return Decision{}, err
	}
	defer g.EndWait(runID)

	select {
	case d := <-ch:
		return d, nil
	case <-ctx.Done():
		return Decision{}, ctx.Err()
	}
}

// HasWaiter reports whether an in-process waiter is registered for runID.
// Used by the job executor so KindHitlResume can wake a workflow Gate without
// competing for the run lease that Execute still holds while blocked.
func (g *Gate) HasWaiter(runID string) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	_, ok := g.waiters[runID]
	g.mu.Unlock()
	return ok
}

// Resume delivers a decision to a waiting Wait call.
// Returns an error if no waiter is registered for runID.
func (g *Gate) Resume(runID string, d Decision) error {
	if g == nil {
		return fmt.Errorf("hitl: nil gate")
	}
	g.mu.Lock()
	ch, ok := g.waiters[runID]
	g.mu.Unlock()
	if !ok {
		return fmt.Errorf("hitl: not waiting for %s", runID)
	}
	select {
	case ch <- d:
		return nil
	default:
		return fmt.Errorf("hitl: resume already delivered for %s", runID)
	}
}
