package main

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
)

func TestResetCredentialsOverrideClearsCredsKeepsKnobs(t *testing.T) {
	st := store.NewMemory()
	base := runtimecfg.Snapshot{}
	h := runtimecfg.New(base)
	steps := 30
	if err := h.ApplyKnobs(context.Background(), st, runtimecfg.KnobsPatch{MaxSteps: &steps}); err != nil {
		t.Fatal(err)
	}
	if err := h.ApplyCreds(context.Background(), st, runtimecfg.CredsPatch{AdminToken: "lost-adm"}); err != nil {
		t.Fatal(err)
	}

	if err := resetCredentialsInStore(context.Background(), st); err != nil {
		t.Fatal(err)
	}

	// reload into a fresh holder (no config baseline here) -> creds override gone
	h2 := runtimecfg.New(runtimecfg.Snapshot{})
	if err := h2.Load(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if h2.Credentials().AdminToken != "" {
		t.Fatalf("cred override must be cleared, got %q", h2.Credentials().AdminToken)
	}
	// knobs override must survive
	if h2.Knobs().MaxSteps != 30 {
		t.Fatalf("knob override must survive reset, got %d", h2.Knobs().MaxSteps)
	}
}
