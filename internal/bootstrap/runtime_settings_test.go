package bootstrap

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
)

func TestActiveGoalBaselineInBuildRuntimeHolder(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{}
	h := buildRuntimeHolder(cfg, st, "op", "adm", nil)
	if !h.Knobs().ActiveGoalEnabled || h.Knobs().ActiveGoalMaxDepth != 8 || h.Knobs().ActiveGoalMaxTextRunes != 500 {
		t.Fatalf("baseline active goal knobs wrong: %+v", h.Knobs())
	}
}

func TestBuildRuntimeHolderBaselineAndOverride(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{}
	cfg.Run.MaxSteps = 16
	cfg.Run.ToolTimeoutSec = 60
	cfg.Conversation.MaxMessages = 40
	cfg.Conversation.CompactThreshold = 0.8

	ops := []controlplane.Operator{{ID: "alice", Token: "ta"}}
	h := buildRuntimeHolder(cfg, st, "op", "adm", ops)
	if h.Knobs().MaxSteps != 16 || h.Credentials().AdminToken != "adm" {
		t.Fatalf("baseline wrong: knobs=%+v admin=%q", h.Knobs(), h.Credentials().AdminToken)
	}
	// Baseline compaction defaults to enabled when config does not disable it.
	if !h.Knobs().CompactionEnabled {
		t.Fatalf("baseline CompactionEnabled must default to true: %+v", h.Knobs())
	}
	if !h.Knobs().MemoryEnabled || !h.Knobs().MemoryAutoExtract {
		t.Fatalf("baseline memory knobs must default to true: %+v", h.Knobs())
	}
	if !h.Knobs().ActiveGoalEnabled || h.Knobs().ActiveGoalMaxDepth != 8 || h.Knobs().ActiveGoalMaxTextRunes != 500 {
		t.Fatalf("baseline active goal knobs wrong: %+v", h.Knobs())
	}
	if !h.Knobs().ContextOffloadEnabled {
		t.Fatalf("baseline ContextOffloadEnabled must default to true: %+v", h.Knobs())
	}

	// persist an override, then rebuild (simulates restart) -> override loaded
	steps := 33
	if err := h.ApplyKnobs(context.Background(), st, runtimecfg.KnobsPatch{MaxSteps: &steps}); err != nil {
		t.Fatal(err)
	}
	h2 := buildRuntimeHolder(cfg, st, "op", "adm", ops)
	if h2.Knobs().MaxSteps != 33 {
		t.Fatalf("override must survive rebuild: %d", h2.Knobs().MaxSteps)
	}
	if h2.Knobs().MaxMessages != 40 {
		t.Fatalf("unset knob must keep config baseline: %d", h2.Knobs().MaxMessages)
	}
	off := false
	if err := h2.ApplyKnobs(context.Background(), st, runtimecfg.KnobsPatch{MemoryEnabled: &off}); err != nil {
		t.Fatal(err)
	}
	h3 := buildRuntimeHolder(cfg, st, "op", "adm", ops)
	if h3.Knobs().MemoryEnabled {
		t.Fatalf("MemoryEnabled override must survive rebuild: %+v", h3.Knobs())
	}
	if !h3.Knobs().MemoryAutoExtract {
		t.Fatalf("unset MemoryAutoExtract must keep baseline true: %+v", h3.Knobs())
	}
}

// Hot-reload must keep ContextOffloadEnabled on when no KV override is present
// (same baseline as startup; zero-value false would silently disable offload).
func TestReloadBaselineKeepsContextOffloadEnabled(t *testing.T) {
	base := runtimeBaseline(config.Config{}, "op", "adm", nil, "json")
	if !base.Knobs.ContextOffloadEnabled {
		t.Fatal("runtimeBaseline must set ContextOffloadEnabled=true")
	}
	h := runtimecfg.New(base)
	// Simulate SIGHUP/reload replacing the YAML baseline with no KV overrides.
	h.ReplaceBaseline(runtimeBaseline(config.Config{}, "op", "adm", nil, "json"))
	if !h.Knobs().ContextOffloadEnabled {
		t.Fatal("after ReplaceBaseline without KV override, ContextOffloadEnabled must stay true")
	}
}
