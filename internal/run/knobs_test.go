package run

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/runtimecfg"
)

type fakeKnobs struct{ k runtimecfg.Knobs }

func (f fakeKnobs) Knobs() runtimecfg.Knobs { return f.k }

func TestEffectiveMaxSteps(t *testing.T) {
	// override wins
	e := &Engine{MaxSteps: 16, Settings: fakeKnobs{k: runtimecfg.Knobs{MaxSteps: 5}}}
	if got := e.effectiveMaxSteps(); got != 5 {
		t.Fatalf("override maxsteps=%d", got)
	}
	// nil settings -> struct field
	e2 := &Engine{MaxSteps: 20}
	if got := e2.effectiveMaxSteps(); got != 20 {
		t.Fatalf("field maxsteps=%d", got)
	}
	// zero everywhere -> default 16
	e3 := &Engine{}
	if got := e3.effectiveMaxSteps(); got != 16 {
		t.Fatalf("default maxsteps=%d", got)
	}
	// snapshot zero (not overridden) falls back to field
	e4 := &Engine{MaxSteps: 20, Settings: fakeKnobs{k: runtimecfg.Knobs{}}}
	if got := e4.effectiveMaxSteps(); got != 20 {
		t.Fatalf("zero snapshot must fall back: %d", got)
	}
}

func TestEffectiveMaxMessages(t *testing.T) {
	e := &Engine{MaxMessages: 40, Settings: fakeKnobs{k: runtimecfg.Knobs{MaxMessages: 12}}}
	if got := e.effectiveMaxMessages(); got != 12 {
		t.Fatalf("override maxmessages=%d", got)
	}
	if got := (&Engine{}).effectiveMaxMessages(); got != 40 {
		t.Fatalf("default maxmessages=%d", got)
	}
}

func TestToolTimeoutHot(t *testing.T) {
	e := &Engine{ToolTimeout: 30 * time.Second, Settings: fakeKnobs{k: runtimecfg.Knobs{ToolTimeout: 5 * time.Second}}}
	if got := e.toolTimeout(); got != 5*time.Second {
		t.Fatalf("override timeout=%v", got)
	}
	e2 := &Engine{ToolTimeout: 30 * time.Second}
	if got := e2.toolTimeout(); got != 30*time.Second {
		t.Fatalf("field timeout=%v", got)
	}
	if got := (&Engine{}).toolTimeout(); got != DefaultToolTimeout {
		t.Fatalf("default timeout=%v", got)
	}
}

func TestCompactorEffectiveCompaction(t *testing.T) {
	// nil settings, zero fields -> enabled + code defaults
	c := &Compactor{}
	en, th, res, keep := c.effectiveCompaction()
	if !en || th != defaultCompactThreshold || res != defaultCompactReserve || keep != defaultCompactKeepRecent {
		t.Fatalf("defaults: en=%v th=%v res=%d keep=%d", en, th, res, keep)
	}
	// explicit disable via snapshot
	c2 := &Compactor{Settings: fakeKnobs{k: runtimecfg.Knobs{CompactionEnabled: false}}}
	if en, _, _, _ := c2.effectiveCompaction(); en {
		t.Fatal("compaction must be disabled by snapshot")
	}
	// threshold/reserve/keep overrides
	c3 := &Compactor{Threshold: 0.8, ReserveTokens: 8000, KeepRecent: 8,
		Settings: fakeKnobs{k: runtimecfg.Knobs{
			CompactionEnabled: true, CompactThreshold: 0.5,
			CompactReserveTokens: 400, CompactKeepRecent: 3}}}
	en, th, res, keep = c3.effectiveCompaction()
	if !en || th != 0.5 || res != 400 || keep != 3 {
		t.Fatalf("overrides: en=%v th=%v res=%d keep=%d", en, th, res, keep)
	}
	// struct fields used when snapshot leaves them zero
	c4 := &Compactor{Threshold: 0.7, ReserveTokens: 1000, KeepRecent: 5,
		Settings: fakeKnobs{k: runtimecfg.Knobs{CompactionEnabled: true}}}
	_, th, res, keep = c4.effectiveCompaction()
	if th != 0.7 || res != 1000 || keep != 5 {
		t.Fatalf("field fallback: th=%v res=%d keep=%d", th, res, keep)
	}
}
