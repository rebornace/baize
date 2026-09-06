package runtimecfg

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/store"
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

func TestApplyKnobsPatchValidates(t *testing.T) {
	h := New(baseSnapshot())
	cases := []struct {
		name string
		p    KnobsPatch
		ok   bool
	}{
		{"steps high", KnobsPatch{MaxSteps: ptr(101)}, false},
		{"steps low", KnobsPatch{MaxSteps: ptr(0)}, false},
		{"messages high", KnobsPatch{MaxMessages: ptr(501)}, false},
		{"timeout high", KnobsPatch{ToolTimeoutSeconds: ptr(601)}, false},
		{"threshold high", KnobsPatch{CompactThreshold: ptr(0.96)}, false},
		{"threshold low", KnobsPatch{CompactThreshold: ptr(0.09)}, false},
		{"reserve low", KnobsPatch{CompactReserveTokens: ptr(200)}, false},
		{"keeprecent high", KnobsPatch{KeepRecent: ptr(101)}, false},
		{"summary timeout high", KnobsPatch{CompactSummaryTimeoutSeconds: ptr(601)}, false},
		{"summary timeout low", KnobsPatch{CompactSummaryTimeoutSeconds: ptr(0)}, false},
		{"valid summary timeout", KnobsPatch{CompactSummaryTimeoutSeconds: ptr(120)}, true},
		{"valid steps", KnobsPatch{MaxSteps: ptr(32)}, true},
		{"valid threshold", KnobsPatch{CompactThreshold: ptr(0.7)}, true},
		{"compaction off", KnobsPatch{CompactionEnabled: ptr(false)}, true},
		{"empty patch", KnobsPatch{}, true},
	}
	for _, tc := range cases {
		err := h.ValidateKnobs(tc.p)
		if tc.ok && err != nil {
			t.Errorf("%s: unexpected err %v", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s: expected validation error", tc.name)
		}
	}
}

func TestApplyKnobsPatchOverlaysAndReports(t *testing.T) {
	h := New(baseSnapshot())
	off := false
	if err := h.ApplyKnobs(context.Background(), nil, KnobsPatch{
		MaxSteps: ptr(32), CompactionEnabled: &off, CompactSummaryTimeoutSeconds: ptr(120),
	}); err != nil {
		t.Fatal(err)
	}
	k := h.Knobs()
	if k.MaxSteps != 32 || k.CompactionEnabled {
		t.Fatalf("overlay not applied: %+v", k)
	}
	if k.CompactSummaryTimeout != 120*time.Second {
		t.Fatalf("summary timeout overlay wrong: %v", k.CompactSummaryTimeout)
	}
	if k.MaxMessages != 40 {
		t.Fatalf("unset field must keep baseline: %d", k.MaxMessages)
	}
	ov := h.KnobsOverride()
	if ov.MaxSteps == nil || *ov.MaxSteps != 32 || ov.CompactionEnabled == nil || *ov.CompactionEnabled {
		t.Fatalf("override state wrong: %+v", ov)
	}
}

func TestApplyCredsPatchRotateAndOperators(t *testing.T) {
	h := New(baseSnapshot())
	ctx := context.Background()
	// rotate admin
	if err := h.ApplyCreds(ctx, nil, CredsPatch{AdminToken: "new-adm"}); err != nil {
		t.Fatal(err)
	}
	if h.Credentials().AdminToken != "new-adm" {
		t.Fatal("admin token not rotated")
	}
	// add operator bob
	if err := h.ApplyCreds(ctx, nil, CredsPatch{AddOperators: []OperatorInput{{ID: "bob", Token: "tb"}}}); err != nil {
		t.Fatal(err)
	}
	if len(h.Credentials().Operators) != 2 {
		t.Fatalf("want 2 operators, got %d", len(h.Credentials().Operators))
	}
	// duplicate id -> conflict
	if err := h.ApplyCreds(ctx, nil, CredsPatch{AddOperators: []OperatorInput{{ID: "bob", Token: "x"}}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate operator id must be ErrConflict, got %v", err)
	}
	// cannot remove a config-baseline operator
	if err := h.ApplyCreds(ctx, nil, CredsPatch{RemoveOperators: []string{"alice"}}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("removing config operator must be ErrBadRequest, got %v", err)
	}
	// removing a non-existent runtime operator -> bad request
	if err := h.ApplyCreds(ctx, nil, CredsPatch{RemoveOperators: []string{"nobody"}}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("removing unknown operator must be ErrBadRequest, got %v", err)
	}
	// remove runtime operator bob ok
	if err := h.ApplyCreds(ctx, nil, CredsPatch{RemoveOperators: []string{"bob"}}); err != nil {
		t.Fatalf("remove runtime operator: %v", err)
	}
	if len(h.Credentials().Operators) != 1 {
		t.Fatalf("want 1 operator after remove, got %d", len(h.Credentials().Operators))
	}
}

func TestApplyCredsResetRestoresBaseline(t *testing.T) {
	h := New(baseSnapshot())
	ctx := context.Background()
	if err := h.ApplyCreds(ctx, nil, CredsPatch{AdminToken: "new-adm"}); err != nil {
		t.Fatal(err)
	}
	if err := h.ApplyCreds(ctx, nil, CredsPatch{Reset: true}); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if h.Credentials().AdminToken != "base-adm" {
		t.Fatalf("reset must restore baseline, got %q", h.Credentials().AdminToken)
	}
	// reset together with another field -> bad request
	if err := h.ApplyCreds(ctx, nil, CredsPatch{Reset: true, AdminToken: "x"}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("reset with other fields must be ErrBadRequest, got %v", err)
	}
	// Note: with merge semantics an empty override slot always keeps the config
	// baseline, so a PATCH can never empty a configured gate (break-glass holds);
	// the lockout guard in ApplyCreds is defense-in-depth and is structurally
	// unreachable via the API.
}

func TestLoadFromStoreAppliesOverride(t *testing.T) {
	st := store.NewMemory()
	raw := []byte(`{"knobs":{"max_steps":24},"creds":{"admin_token":"kv-adm"}}`)
	if err := st.UpsertSetting(store.SettingKeyRuntimeSettings, raw); err != nil {
		t.Fatal(err)
	}
	h := New(baseSnapshot())
	if err := h.Load(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if h.Knobs().MaxSteps != 24 || h.Credentials().AdminToken != "kv-adm" {
		t.Fatalf("KV override not loaded: knobs=%+v", h.Knobs())
	}
	if h.Knobs().MaxMessages != 40 {
		t.Fatalf("unset knob must keep baseline: %d", h.Knobs().MaxMessages)
	}
}

func TestLoadCorruptJSONFallsBack(t *testing.T) {
	st := store.NewMemory()
	_ = st.UpsertSetting(store.SettingKeyRuntimeSettings, []byte(`{not json`))
	h := New(baseSnapshot())
	if err := h.Load(context.Background(), st); err != nil {
		t.Fatalf("corrupt JSON must not error: %v", err)
	}
	if h.Knobs().MaxSteps != 16 {
		t.Fatalf("corrupt KV must fall back to baseline: %d", h.Knobs().MaxSteps)
	}
}

func TestCredentialsViewMasksTokens(t *testing.T) {
	h := New(baseSnapshot())
	if err := h.ApplyCreds(context.Background(), nil, CredsPatch{
		AdminToken:   "rotated-adm",
		AddOperators: []OperatorInput{{ID: "bob", Token: "tb"}},
	}); err != nil {
		t.Fatal(err)
	}
	v := h.CredentialsView()
	if v.Source != "override" {
		t.Fatalf("source=%s want override", v.Source)
	}
	if !v.OperatorSet || !v.AdminSet {
		t.Fatalf("slots should be set: %+v", v)
	}
	// alice from config (baseline), bob from runtime
	src := map[string]string{}
	for _, op := range v.Operators {
		src[op.ID] = op.Source
	}
	if src["alice"] != "config" || src["bob"] != "runtime" {
		t.Fatalf("operator sources wrong: %+v", v.Operators)
	}
	// the view must never carry a token: marshal and assert no secret substring
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "rotated-adm") || strings.Contains(string(b), "tb") || strings.Contains(string(b), "ta") {
		t.Fatalf("credentials view leaked a token: %s", b)
	}
}

func TestKnobsViewMarksOverridden(t *testing.T) {
	h := New(baseSnapshot())
	_ = h.ApplyKnobs(context.Background(), nil, KnobsPatch{MaxSteps: ptr(32)})
	v := h.KnobsView()
	if v.Effective.MaxSteps != 32 || !v.Overridden.MaxSteps {
		t.Fatalf("maxsteps should be effective+overridden: %+v", v)
	}
	if v.Overridden.MaxMessages || v.Effective.MaxMessages != 40 {
		t.Fatalf("maxmessages should be baseline not overridden: %+v", v)
	}
}

func ptr[T any](v T) *T { return &v }
