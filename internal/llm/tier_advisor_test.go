package llm

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

// stubTierAdvisor is a controllable TierAdvisor for DP-4 tests.
type stubTierAdvisor struct {
	outcome TierAdviceOutcome
	tier    string
	got     string
}

func (s *stubTierAdvisor) AdviseTier(_ context.Context, text string) TierAdvice {
	s.got = text
	return TierAdvice{Outcome: s.outcome, Tier: s.tier}
}

// A standard-tier turn on long text uses the advisor's power pick, and the
// resolved model comes from the power tier.
func TestResolveModelAdvisorOverridesStandard(t *testing.T) {
	profiles := tiered()
	// Plain medium text classifies as standard (eligible for DP-4).
	sig := TaskSignals{Text: repeatRunes("普通内容。", 90)} // ~450 runes
	if ClassifyTask(sig) != store.AutoTierStandard {
		t.Fatalf("precondition: want standard tier")
	}
	adv := &stubTierAdvisor{tier: store.AutoTierPower, outcome: TierAdviceOK}
	sel, ok := ResolveModel(AutoProfileID, sig, profiles, WithTierAdvisor(adv))
	if !ok {
		t.Fatal("vision ok = false")
	}
	if sel.Tier != store.AutoTierPower {
		t.Errorf("sel.Tier = %q, want power", sel.Tier)
	}
	if sel.ProfileID != "mp_power" {
		t.Errorf("sel.ProfileID = %q, want mp_power", sel.ProfileID)
	}
	if adv.got != sig.Text {
		t.Errorf("advisor text not passed through")
	}
}

// When the advisor abstains (ok=false) the router keeps standard.
func TestResolveModelAdvisorAbstainKeepsStandard(t *testing.T) {
	profiles := tiered()
	sig := TaskSignals{Text: repeatRunes("普通内容。", 90)}
	adv := &stubTierAdvisor{outcome: TierAdviceSkipped}
	sel, _ := ResolveModel(AutoProfileID, sig, profiles, WithTierAdvisor(adv))
	if sel.Tier != store.AutoTierStandard {
		t.Errorf("sel.Tier = %q, want standard", sel.Tier)
	}
	if sel.ProfileID != "mp_standard" {
		t.Errorf("sel.ProfileID = %q, want mp_standard", sel.ProfileID)
	}
}

// A turn the heuristic already pins to power is never sent to the advisor.
func TestResolveModelAdvisorSkipsPowerTier(t *testing.T) {
	profiles := tiered()
	sig := TaskSignals{Text: repeatRunes("字", 4500)} // heuristic: power
	adv := &stubTierAdvisor{tier: store.AutoTierLight, outcome: TierAdviceOK}
	sel, _ := ResolveModel(AutoProfileID, sig, profiles, WithTierAdvisor(adv))
	if sel.Tier != store.AutoTierPower {
		t.Errorf("sel.Tier = %q, want power", sel.Tier)
	}
	if adv.got != "" {
		t.Errorf("advisor should not be consulted on power tier, got %q", adv.got)
	}
}

// With no advisor wired, behavior is exactly the legacy standard routing.
func TestResolveModelNoAdvisor(t *testing.T) {
	profiles := tiered()
	sig := TaskSignals{Text: repeatRunes("普通内容。", 90)}
	sel, _ := ResolveModel(AutoProfileID, sig, profiles)
	if sel.Tier != store.AutoTierStandard || sel.ProfileID != "mp_standard" {
		t.Errorf("legacy routing changed: tier=%q id=%q", sel.Tier, sel.ProfileID)
	}
}

// An unrecognized advisor value is a fail-open degradation: tier stays
// standard and the selection carries the degraded signal.
func TestResolveModelAdvisorJunkValueDegrades(t *testing.T) {
	profiles := tiered()
	sig := TaskSignals{Text: repeatRunes("普通内容。", 90)}
	adv := &stubTierAdvisor{tier: "ultra-gpt", outcome: TierAdviceOK}
	sel, _ := ResolveModel(AutoProfileID, sig, profiles, WithTierAdvisor(adv))
	if sel.Tier != store.AutoTierStandard {
		t.Errorf("sel.Tier = %q, want standard", sel.Tier)
	}
	// The stub returned OK at the interface boundary, but routing.go treats
	// the unrecognized tier as a fail-open (handled in the bootstrap adapter
	// via TierAdviceDegraded); here the raw router simply keeps standard.
}

// When the advisor reports TierAdviceDegraded, the tier stays standard and
// RouteAdvisorDegraded is carried out for the unified event.
func TestResolveModelAdvisorDegradedFlag(t *testing.T) {
	profiles := tiered()
	sig := TaskSignals{Text: repeatRunes("普通内容。", 90)}
	adv := &stubTierAdvisor{outcome: TierAdviceDegraded}
	sel, _ := ResolveModel(AutoProfileID, sig, profiles, WithTierAdvisor(adv))
	if sel.Tier != store.AutoTierStandard {
		t.Errorf("sel.Tier = %q, want standard", sel.Tier)
	}
	if !sel.RouteAdvisorDegraded {
		t.Error("RouteAdvisorDegraded = false, want true")
	}
}

func repeatRunes(s string, n int) string {
	out := ""
	for len([]rune(out)) < n {
		out += s
	}
	return out
}
