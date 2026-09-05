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

func TestResetCredentialsCorruptKVRefusesToOverwrite(t *testing.T) {
	st := store.NewMemory()
	corrupt := []byte("{bad json")
	if err := st.UpsertSetting(store.SettingKeyRuntimeSettings, corrupt); err != nil {
		t.Fatal(err)
	}

	err := resetCredentialsInStore(context.Background(), st)
	if err == nil {
		t.Fatal("expected error for corrupt runtime_settings KV, got nil")
	}

	// The corrupt blob must be left untouched (non-destructive).
	got, ok, err := st.GetSetting(store.SettingKeyRuntimeSettings)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("runtime_settings KV must still exist after refused reset")
	}
	if string(got) != string(corrupt) {
		t.Fatalf("corrupt KV must be left untouched, got %q", string(got))
	}
}
