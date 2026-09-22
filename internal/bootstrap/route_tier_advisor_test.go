package bootstrap

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
)

// stubAsk is a minimal decide.Ask returning a fixed Answer.
type stubAsk struct {
	ans decide.Answer
	err error
}

func (s *stubAsk) Enabled() bool { return true }
func (s *stubAsk) Ask(_ context.Context, _ decide.Question) (decide.Answer, error) {
	return s.ans, s.err
}

func newAdvisor(t *testing.T, k runtimecfg.Knobs, ask decide.Ask) *routeTierAdvisor {
	t.Helper()
	h := runtimecfg.New(runtimecfg.Snapshot{Knobs: k})
	return &routeTierAdvisor{settings: h, decider: ask}
}

// Enabled switches + long text + power answer -> returns power.
func TestRouteTierAdvisorPower(t *testing.T) {
	k := runtimecfg.Knobs{
		DecideEnabled: true, DecideRouteEnabled: true, DecideRouteMinRunes: 10,
	}
	ask := &stubAsk{ans: decide.Answer{Value: store.AutoTierPower, Source: decide.SourceRemote}}
	a := newAdvisor(t, k, ask)
	got, ok := a.AdviseTier(context.Background(), "这是一段足够长的请求文本用来触发档位判断")
	if !ok || got != store.AutoTierPower {
		t.Fatalf("got=%q ok=%v want power", got, ok)
	}
}

// Short text under the floor is rejected without consulting the decider.
func TestRouteTierAdvisorShortRejected(t *testing.T) {
	k := runtimecfg.Knobs{
		DecideEnabled: true, DecideRouteEnabled: true, DecideRouteMinRunes: 1000,
	}
	called := false
	ask := &recordingAsk{record: &called}
	a := newAdvisor(t, k, ask)
	if _, ok := a.AdviseTier(context.Background(), "短"); ok {
		t.Fatal("short turn must not be advised")
	}
	if called {
		t.Fatal("decider must not be called for a short turn")
	}
}

// Master or route switch off -> abstain.
func TestRouteTierAdvisorSwitchesOff(t *testing.T) {
	ask := &stubAsk{ans: decide.Answer{Value: store.AutoTierPower}}
	cases := []runtimecfg.Knobs{
		{DecideRouteEnabled: true, DecideRouteMinRunes: 1}, // master off
		{DecideEnabled: true, DecideRouteMinRunes: 1},      // route off
	}
	for i, k := range cases {
		a := newAdvisor(t, k, ask)
		if _, ok := a.AdviseTier(context.Background(), "这是一段足够长的文本内容用来触发判断"); ok {
			t.Fatalf("case %d: must abstain when a switch is off", i)
		}
	}
}

// Degraded answer -> abstain (keep standard).
func TestRouteTierAdvisorDegraded(t *testing.T) {
	k := runtimecfg.Knobs{
		DecideEnabled: true, DecideRouteEnabled: true, DecideRouteMinRunes: 1,
	}
	ask := &stubAsk{ans: decide.Answer{Value: store.AutoTierPower, Degraded: true}}
	a := newAdvisor(t, k, ask)
	if _, ok := a.AdviseTier(context.Background(), "这是一段足够长的文本内容用来触发判断"); ok {
		t.Fatal("degraded answer must be rejected")
	}
}

// recordingAsk tracks whether Ask was invoked.
type recordingAsk struct {
	record *bool
}

func (s *recordingAsk) Enabled() bool { return true }
func (s *recordingAsk) Ask(_ context.Context, _ decide.Question) (decide.Answer, error) {
	*s.record = true
	return decide.Answer{Value: store.AutoTierPower}, nil
}
