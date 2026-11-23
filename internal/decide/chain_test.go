package decide

import (
	"context"
	"errors"
	"testing"
)

// stubAsk is a controllable Ask for Chain tests.
type stubAsk struct {
	enabled bool
	answer  Answer
	err     error
	calls   int
}

func (s *stubAsk) Enabled() bool { return s.enabled }

func (s *stubAsk) Ask(_ context.Context, _ Question) (Answer, error) {
	s.calls++
	return s.answer, s.err
}

func TestChainSkipsNilAndDisabled(t *testing.T) {
	good := &stubAsk{enabled: true, answer: Answer{Verdict: VerdictYes, Source: "good"}}
	ch := NewChain(nil, &stubAsk{enabled: false}, good)
	ans, _ := ch.Ask(context.Background(), Question{OnFail: VerdictNo})
	if ans.Verdict != VerdictYes || ans.Source != "good" {
		t.Fatalf("ans=%+v want yes/good", ans)
	}
	if good.calls != 1 {
		t.Fatalf("good calls=%d want 1", good.calls)
	}
}

func TestChainReturnsFirstSuccess(t *testing.T) {
	first := &stubAsk{enabled: true, answer: Answer{Verdict: VerdictNo, Source: "first"}}
	second := &stubAsk{enabled: true, answer: Answer{Verdict: VerdictYes, Source: "second"}}
	ch := NewChain(first, second)
	ans, _ := ch.Ask(context.Background(), Question{OnFail: VerdictYes})
	if ans.Verdict != VerdictNo || ans.Source != "first" {
		t.Fatalf("ans=%+v want no/first", ans)
	}
	if second.calls != 0 {
		t.Fatalf("second must not be consulted, calls=%d", second.calls)
	}
}

func TestChainDefersOnUnavailable(t *testing.T) {
	// A provider that abstains must let the chain try the next implementation.
	deferring := &stubAsk{enabled: true, err: ErrUnavailable}
	good := &stubAsk{enabled: true, answer: Answer{Verdict: VerdictYes, Source: "good"}}
	ch := NewChain(deferring, good)
	ans, _ := ch.Ask(context.Background(), Question{OnFail: VerdictNo})
	if ans.Verdict != VerdictYes || ans.Source != "good" {
		t.Fatalf("ans=%+v want yes/good", ans)
	}
}

func TestChainFallsBackWhenAllFail(t *testing.T) {
	bad := &stubAsk{enabled: true, err: errors.New("boom")}
	unavail := &stubAsk{enabled: true, err: ErrUnavailable}
	ch := NewChain(bad, unavail)
	ans, _ := ch.Ask(context.Background(), Question{OnFail: VerdictYes})
	if ans.Verdict != VerdictYes {
		t.Fatalf("verdict=%v want OnFail yes", ans.Verdict)
	}
	if !ans.Degraded || ans.Source != SourceFallback {
		t.Fatalf("ans=%+v want degraded/fallback", ans)
	}
}

func TestChainEmptyFallsBack(t *testing.T) {
	ch := NewChain()
	ans, _ := ch.Ask(context.Background(), Question{OnFail: VerdictNo})
	if ans.Verdict != VerdictNo || !ans.Degraded {
		t.Fatalf("ans=%+v want no/degraded", ans)
	}
}
