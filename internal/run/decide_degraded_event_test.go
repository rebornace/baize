package run

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/decide"
)

// When the DP-1 layer fails open (degraded), a unified decide.degraded event
// must be emitted once, carrying the decision-point kind; the extraction still
// proceeds (fail open).
func TestDP1EmitsUnifiedDegradedEvent(t *testing.T) {
	decider := &stubDecider{ans: decide.Answer{
		Verdict: decide.VerdictYes, Source: decide.SourceFallback, Degraded: true,
	}}
	eng, r, calls := newDP1Engine(t, decider, decideSettings())

	eng.maybeExtractMemory(context.Background(), r.ID, "alice", "今天我把 VPN 弄好了", "辛苦了")

	// Fail open: extraction still ran.
	if *calls != 1 {
		t.Fatalf("degraded must fail open and still extract, calls=%d want 1", *calls)
	}

	evs, _ := eng.Store.ListEvents(r.ID)
	n := 0
	for _, ev := range evs {
		if ev.Type != EventDecideDegraded {
			continue
		}
		n++
		if ev.Data["kind"] != decide.KindMemoryExtract {
			t.Fatalf("kind=%v want %s", ev.Data["kind"], decide.KindMemoryExtract)
		}
		if ev.Data["source"] != decide.SourceFallback {
			t.Fatalf("source=%v want %s", ev.Data["source"], decide.SourceFallback)
		}
	}
	if n != 1 {
		t.Fatalf("decide.degraded count=%d want 1", n)
	}
}
