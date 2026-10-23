package run

import (
	"context"
	"errors"
	"testing"
	"time"
)

// resumeAfterWait drains Execute (expect ErrWaitingHuman) then ContinueFromHITL.
func resumeAfterWait(t *testing.T, eng *Engine, errCh <-chan error, runID string, d Decision) error {
	t.Helper()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, ErrWaitingHuman) {
			t.Fatalf("Execute: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Execute timed out waiting for HITL")
	}
	return eng.ContinueFromHITL(context.Background(), runID, d)
}
