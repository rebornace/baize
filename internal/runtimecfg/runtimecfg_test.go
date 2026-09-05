package runtimecfg

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
)

func baseSnapshot() Snapshot {
	return Snapshot{
		Knobs: Knobs{
			MaxMessages: 40, MaxSteps: 16,
			ToolTimeout: 60 * time.Second, CompactionEnabled: true,
			CompactThreshold: 0.8, CompactReserveTokens: 8000, CompactKeepRecent: 8,
		},
		Creds: Credentials{
			OperatorToken: "base-op", AdminToken: "base-adm",
			Operators: []controlplane.Operator{{ID: "alice", Token: "ta"}},
		},
	}
}

func TestNewHolderExposesBaseline(t *testing.T) {
	h := New(baseSnapshot())
	k := h.Knobs()
	if k.MaxSteps != 16 || k.MaxMessages != 40 || k.ToolTimeout != 60*time.Second {
		t.Fatalf("baseline knobs wrong: %+v", k)
	}
	if !k.CompactionEnabled {
		t.Fatal("compaction should default to enabled from baseline")
	}
	c := h.Credentials()
	if c.OperatorToken != "base-op" || c.AdminToken != "base-adm" || len(c.Operators) != 1 {
		t.Fatalf("baseline creds wrong: %+v", c)
	}
}

func TestNilHolderSafe(t *testing.T) {
	var h *Holder
	if h.Knobs() != (Knobs{}) {
		t.Fatal("nil holder Knobs must be zero value")
	}
	if got := h.Credentials(); got.OperatorToken != "" || len(got.Operators) != 0 {
		t.Fatalf("nil holder creds must be zero: %+v", got)
	}
}

func TestMergeSnapshotOverlaysOnlyProvided(t *testing.T) {
	base := baseSnapshot()
	steps := 24
	off := false
	snap := mergeSnapshot(base,
		knobsOverride{MaxSteps: &steps, CompactionEnabled: &off},
		credsOverride{AdminToken: "new-adm"})
	if snap.Knobs.MaxSteps != 24 {
		t.Fatalf("maxsteps override: %d", snap.Knobs.MaxSteps)
	}
	if snap.Knobs.MaxMessages != 40 {
		t.Fatalf("unset knob must keep baseline: %d", snap.Knobs.MaxMessages)
	}
	if snap.Knobs.CompactionEnabled {
		t.Fatal("compaction should be overridden to false")
	}
	if snap.Creds.AdminToken != "new-adm" {
		t.Fatalf("admin override: %q", snap.Creds.AdminToken)
	}
	if snap.Creds.OperatorToken != "base-op" {
		t.Fatalf("unset operator token must keep baseline: %q", snap.Creds.OperatorToken)
	}
	// baseline operator survives; runtime operators appended
	if len(snap.Creds.Operators) != 1 || snap.Creds.Operators[0].ID != "alice" {
		t.Fatalf("base operators must survive: %+v", snap.Creds.Operators)
	}
}

func TestMergeAppendsRuntimeOperators(t *testing.T) {
	base := baseSnapshot()
	snap := mergeSnapshot(base, knobsOverride{}, credsOverride{
		Operators: []operatorEntry{{ID: "bob", Token: "tb"}},
	})
	if len(snap.Creds.Operators) != 2 {
		t.Fatalf("want base+runtime = 2 operators, got %+v", snap.Creds.Operators)
	}
}
