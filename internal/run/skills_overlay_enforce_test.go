package run

import (
	"context"
	"strconv"
	"testing"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/tool"
)

// newScopedEngine builds an Engine with a default skill "demo" that declares
// only list_tickets (connector ticket-api), that connector's login/me auth
// primitives, and enough unrelated connector tools to push the catalog above
// the default routing threshold (12). The union-vs-scope decision is taken in
// specsForRun before any DP-2a narrowing.
func newScopedEngine(t *testing.T, routing bool) (*Engine, string) {
	t.Helper()
	cat := loadTestCatalog(t, map[string]struct {
		desc  string
		tools []string
		body  string
	}{
		"demo": {desc: "tickets", tools: []string{"list_tickets"}, body: "use list_tickets"},
	})

	reg := tool.NewRegistry()
	meta := func(name, source string) tool.Meta {
		return tool.Meta{
			Spec:        llm.ToolSpec{Name: name, Description: name},
			ConnectorID: source,
		}
	}
	noop := func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	}
	reg.RegisterMeta(meta("list_tickets", "ticket-api"), noop, false)
	reg.RegisterMeta(meta("AuthController_login", "ticket-api"), noop, false)
	reg.RegisterMeta(meta("AuthController_me", "ticket-api"), noop, false)
	// 15 unrelated tools across other systems push total above threshold 12.
	for i := 0; i < 15; i++ {
		source := "other-api"
		if i%2 == 0 {
			source = "third-api"
		}
		reg.RegisterMeta(meta("unrelated_"+strconv.Itoa(i), source), noop, false)
	}

	settings := fakeKnobs{k: runtimecfg.Knobs{
		DecideEnabled:            true,
		DecideToolRoutingEnabled: routing,
	}}
	eng := &Engine{Tools: reg, Skills: cat, Settings: settings, Decider: stubAsk{}}
	eng.beginRunSkills("run_1", []string{"demo"}, "sys")
	return eng, "run_1"
}

// stubAsk is a no-op decide.Ask: specsForRun only needs a non-nil Decider for
// its routing gate; narrowTools is not exercised here.
type stubAsk struct{}

func (stubAsk) Enabled() bool { return true }

func (stubAsk) Ask(_ context.Context, _ decide.Question) (decide.Answer, error) {
	return decide.Answer{}, nil
}

func specSet(specs []llm.ToolSpec) map[string]bool {
	out := make(map[string]bool, len(specs))
	for _, s := range specs {
		out[s.Name] = true
	}
	return out
}

// TestSpecsForRunRoutingOnSkipsUnionAndKeepsAuthFloor: with routing on the
// unrelated connector tools must not be unioned back, the skill-scoped tool is
// kept, and the auth floor restores that same system's login/me so the model
// can recover from a 401 even though the skill did not declare them.
func TestSpecsForRunRoutingOnSkipsUnionAndKeepsAuthFloor(t *testing.T) {
	eng, runID := newScopedEngine(t, true)
	got := specSet(eng.specsForRun(runID))

	if !got["list_tickets"] {
		t.Fatalf("skill-scoped tool missing: %v", got)
	}
	if !got["AuthController_login"] || !got["AuthController_me"] {
		t.Fatalf("auth floor missing login/me: %v", got)
	}
	for name := range got {
		if len(name) >= 10 && name[:10] == "unrelated_" {
			t.Fatalf("unrelated connector tool leaked via union: %s", name)
		}
	}
}

// TestSpecsForRunRoutingOffKeepsLegacyUnion: with the routing switch off the
// fail-open union stays so every connector tool still reaches the model.
func TestSpecsForRunRoutingOffKeepsLegacyUnion(t *testing.T) {
	eng, runID := newScopedEngine(t, false)
	got := specSet(eng.specsForRun(runID))

	if !got["list_tickets"] || !got["unrelated_0"] || !got["unrelated_14"] {
		t.Fatalf("routing off must keep the full union; got %v", got)
	}
}

// TestSpecsForRunMasterOffKeepsLegacyUnion: with the master switch off the
// union stays even though the routing switch itself is on.
func TestSpecsForRunMasterOffKeepsLegacyUnion(t *testing.T) {
	eng, runID := newScopedEngine(t, true)
	eng.Settings = fakeKnobs{k: runtimecfg.Knobs{DecideEnabled: false}}
	got := specSet(eng.specsForRun(runID))

	if !got["list_tickets"] || !got["unrelated_0"] {
		t.Fatalf("master off must keep the full union; got %v", got)
	}
}
