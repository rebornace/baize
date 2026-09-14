package runtimecfg

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/settingscrypto"
	"github.com/rebornace/baize/internal/store"
)

const testSettingsKey = "test-settings-key-32bytes-ok!!"

func setTestSettingsKey(t *testing.T) {
	t.Helper()
	t.Setenv("BAIZE_SETTINGS_KEY", testSettingsKey)
}

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
	if h.PublicBaseURL() != "" || h.PublicBaseURLOverridden() {
		t.Fatal("nil holder PublicBaseURL must be empty")
	}
	h.ReplaceBaseline(baseSnapshot()) // must not panic
}

func TestReplaceBaselineKeepsOverrides(t *testing.T) {
	h := New(Snapshot{Knobs: Knobs{MaxSteps: 10, MaxMessages: 40}})
	steps := 20
	h.mu.Lock()
	h.ko = knobsOverride{MaxSteps: &steps}
	h.swapLocked()
	h.mu.Unlock()

	h.ReplaceBaseline(Snapshot{Knobs: Knobs{MaxSteps: 50, MaxMessages: 99}})
	k := h.Knobs()
	if k.MaxSteps != 20 {
		t.Fatalf("override lost: got %d", k.MaxSteps)
	}
	if k.MaxMessages != 99 {
		t.Fatalf("baseline MaxMessages not applied: %d", k.MaxMessages)
	}
}

func TestMergeSnapshotOverlaysOnlyProvided(t *testing.T) {
	base := baseSnapshot()
	steps := 24
	off := false
	snap := mergeSnapshot(base,
		knobsOverride{MaxSteps: &steps, CompactionEnabled: &off},
		credsOverride{AdminToken: "new-adm"}, nil)
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
	}, nil)
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

func TestApplyCredsSealsTokensAtRest(t *testing.T) {
	setTestSettingsKey(t)
	st := store.NewMemory()
	h := New(baseSnapshot())
	ctx := context.Background()
	if err := h.ApplyCreds(ctx, st, CredsPatch{
		OperatorToken: "op-secret",
		AdminToken:    "adm-secret",
		AddOperators:  []OperatorInput{{ID: "bob", Token: "tb-secret"}},
	}); err != nil {
		t.Fatal(err)
	}
	c := h.Credentials()
	if c.OperatorToken != "op-secret" || c.AdminToken != "adm-secret" {
		t.Fatalf("snapshot must stay plaintext: %+v", c)
	}
	var bobTok string
	for _, op := range c.Operators {
		if op.ID == "bob" {
			bobTok = op.Token
		}
	}
	if bobTok != "tb-secret" {
		t.Fatalf("runtime operator token in snapshot: %q", bobTok)
	}

	raw, ok, err := st.GetSetting(store.SettingKeyRuntimeSettings)
	if err != nil || !ok {
		t.Fatalf("get runtime_settings: ok=%v err=%v", ok, err)
	}
	var stored struct {
		Creds credsOverride `json:"creds"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if !settingscrypto.IsSealed(stored.Creds.OperatorToken) {
		t.Fatalf("operator_token at rest must be sealed, got %q", stored.Creds.OperatorToken)
	}
	if !settingscrypto.IsSealed(stored.Creds.AdminToken) {
		t.Fatalf("admin_token at rest must be sealed, got %q", stored.Creds.AdminToken)
	}
	if len(stored.Creds.Operators) != 1 || !settingscrypto.IsSealed(stored.Creds.Operators[0].Token) {
		t.Fatalf("operator entry token at rest must be sealed: %+v", stored.Creds.Operators)
	}
}

func TestApplyKnobsDoesNotSealInMemoryCreds(t *testing.T) {
	setTestSettingsKey(t)
	st := store.NewMemory()
	h := New(baseSnapshot())
	ctx := context.Background()
	if err := h.ApplyCreds(ctx, st, CredsPatch{
		AddOperators: []OperatorInput{{ID: "bob", Token: "tb-secret"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.ApplyKnobs(ctx, st, KnobsPatch{MaxSteps: ptr(20)}); err != nil {
		t.Fatal(err)
	}
	for _, op := range h.Credentials().Operators {
		if op.ID == "bob" {
			if settingscrypto.IsSealed(op.Token) || op.Token != "tb-secret" {
				t.Fatalf("knobs persist must not seal in-memory operator token: %q", op.Token)
			}
		}
	}
	raw, ok, err := st.GetSetting(store.SettingKeyRuntimeSettings)
	if err != nil || !ok {
		t.Fatal(err)
	}
	var stored struct {
		Creds credsOverride `json:"creds"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Creds.Operators) != 1 || !settingscrypto.IsSealed(stored.Creds.Operators[0].Token) {
		t.Fatalf("KV operator token must stay sealed: %+v", stored.Creds.Operators)
	}
}

func TestApplyCredsRequiresSettingsKey(t *testing.T) {
	st := store.NewMemory()
	h := New(baseSnapshot())
	err := h.ApplyCreds(context.Background(), st, CredsPatch{AdminToken: "new-adm"})
	if !errors.Is(err, settingscrypto.ErrNoKey) {
		t.Fatalf("persist creds without BAIZE_SETTINGS_KEY: got %v want ErrNoKey", err)
	}
}

func TestLoadPlaintextCredsBackwardCompat(t *testing.T) {
	st := store.NewMemory()
	raw := []byte(`{"creds":{"admin_token":"kv-adm","operator_token":"kv-op","operators":[{"id":"bob","token":"tb"}]}}`)
	if err := st.UpsertSetting(store.SettingKeyRuntimeSettings, raw); err != nil {
		t.Fatal(err)
	}
	h := New(baseSnapshot())
	if err := h.Load(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	c := h.Credentials()
	if c.AdminToken != "kv-adm" || c.OperatorToken != "kv-op" {
		t.Fatalf("plain creds load: %+v", c)
	}
	var foundBob bool
	for _, op := range c.Operators {
		if op.ID == "bob" && op.Token == "tb" {
			foundBob = true
		}
	}
	if !foundBob {
		t.Fatalf("plain operator token load: %+v", c.Operators)
	}
}

func TestLoadSealedCredsRoundTrip(t *testing.T) {
	setTestSettingsKey(t)
	st := store.NewMemory()
	ctx := context.Background()
	w := New(baseSnapshot())
	if err := w.ApplyCreds(ctx, st, CredsPatch{
		AdminToken: "sealed-adm",
	}); err != nil {
		t.Fatal(err)
	}
	h := New(baseSnapshot())
	if err := h.Load(ctx, st); err != nil {
		t.Fatal(err)
	}
	if h.Credentials().AdminToken != "sealed-adm" {
		t.Fatalf("sealed creds round-trip: %q", h.Credentials().AdminToken)
	}
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

func TestPublicBaseURLBaselineAndOverride(t *testing.T) {
	h := New(Snapshot{PublicBaseURL: "http://yaml.example:8080"})
	if h.PublicBaseURL() != "http://yaml.example:8080" || h.PublicBaseURLOverridden() {
		t.Fatalf("baseline: url=%q overridden=%v", h.PublicBaseURL(), h.PublicBaseURLOverridden())
	}
	url := "http://127.0.0.1:8080/"
	if err := h.ApplyKnobs(context.Background(), nil, KnobsPatch{PublicBaseURL: &url}); err != nil {
		t.Fatal(err)
	}
	if h.PublicBaseURL() != "http://127.0.0.1:8080" {
		t.Fatalf("want normalized override, got %q", h.PublicBaseURL())
	}
	if !h.PublicBaseURLOverridden() {
		t.Fatal("expected overridden")
	}
	clear := ""
	if err := h.ApplyKnobs(context.Background(), nil, KnobsPatch{PublicBaseURL: &clear}); err != nil {
		t.Fatal(err)
	}
	if h.PublicBaseURL() != "http://yaml.example:8080" || h.PublicBaseURLOverridden() {
		t.Fatalf("clear must restore YAML: url=%q overridden=%v", h.PublicBaseURL(), h.PublicBaseURLOverridden())
	}
}

func TestPublicBaseURLValidate(t *testing.T) {
	h := New(Snapshot{})
	bad := "ftp://x"
	if err := h.ValidateKnobs(KnobsPatch{PublicBaseURL: &bad}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("want ErrBadRequest, got %v", err)
	}
	rel := "/relative"
	if err := h.ValidateKnobs(KnobsPatch{PublicBaseURL: &rel}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("relative must fail, got %v", err)
	}
}

func TestPublicBaseURLPersists(t *testing.T) {
	setTestSettingsKey(t)
	st := store.NewMemory()
	h := New(Snapshot{PublicBaseURL: "http://yaml.example"})
	url := "https://runtime.example"
	if err := h.ApplyKnobs(context.Background(), st, KnobsPatch{PublicBaseURL: &url}); err != nil {
		t.Fatal(err)
	}
	h2 := New(Snapshot{PublicBaseURL: "http://yaml.example"})
	if err := h2.Load(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if h2.PublicBaseURL() != "https://runtime.example" || !h2.PublicBaseURLOverridden() {
		t.Fatalf("reload: url=%q overridden=%v", h2.PublicBaseURL(), h2.PublicBaseURLOverridden())
	}
}

func ptr[T any](v T) *T { return &v }
