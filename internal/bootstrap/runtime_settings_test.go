package bootstrap

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
)

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
}
