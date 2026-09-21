package run

import (
	"context"
	"errors"
	"testing"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/memory"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
)

// stubDecider is a controllable decide.Ask for DP-1 tests.
type stubDecider struct {
	ans decide.Answer
	err error
	got decide.Question
}

func (d *stubDecider) Enabled() bool { return true }

func (d *stubDecider) Ask(_ context.Context, q decide.Question) (decide.Answer, error) {
	d.got = q
	return d.ans, d.err
}

func decideSettings() fakeKnobs {
	return fakeKnobs{k: runtimecfg.Knobs{
		MemoryEnabled: true, MemoryAutoExtract: true,
		DecideEnabled: true, DecideMemoryEnabled: true,
	}}
}

func newDP1Engine(t *testing.T, decider decide.Ask, settings fakeKnobs) (*Engine, *store.Run, *int) {
	t.Helper()
	st := store.NewMemory()
	mem := memory.NewMemoryStore()
	calls := 0
	llmStub := &captureLLM{onChat: func(_ []llm.Message, _ []llm.ToolSpec) llm.Message {
		calls++
		return llm.Message{Content: `[{"key":"tea","text":"喜欢绿茶"}]`}
	}}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{
		Store: st, LLM: llmStub, Memory: mem, Decider: decider, Settings: settings,
	}
	return eng, r, &calls
}

// AC-03: a non-degraded No must skip the extraction Chat and record
// memory.extract_skipped with reason=decider_no.
func TestDP1DeciderNoSkipsExtract(t *testing.T) {
	decider := &stubDecider{ans: decide.Answer{Verdict: decide.VerdictNo, Source: decide.SourceRules}}
	eng, r, calls := newDP1Engine(t, decider, decideSettings())

	eng.maybeExtractMemory(context.Background(), r.ID, "alice", "你好", "你好呀")

	if *calls != 0 {
		t.Fatalf("extraction Chat must be skipped, calls=%d want 0", *calls)
	}
	evs, err := eng.Store.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	var skipped *store.Event
	for i := range evs {
		if evs[i].Type == EventMemoryExtractSkipped {
			skipped = &evs[i]
		}
	}
	if skipped == nil {
		t.Fatalf("missing memory.extract_skipped event; evs=%+v", evs)
	}
	if skipped.Data["reason"] != "decider_no" {
		t.Fatalf("reason=%v want decider_no", skipped.Data["reason"])
	}
	list, _ := eng.Memory.List("alice", 20, 0)
	if len(list) != 0 {
		t.Fatalf("no memory must be written on No; list=%+v", list)
	}
}

// AC-04: a degraded verdict (all providers failed, OnFail=Yes) must extract as
// today — fail open rather than silently dropping a memory.
func TestDP1DeciderDegradedStillExtracts(t *testing.T) {
	decider := &stubDecider{ans: decide.Answer{
		Verdict: decide.VerdictYes, Source: decide.SourceFallback, Degraded: true,
	}}
	eng, r, calls := newDP1Engine(t, decider, decideSettings())

	eng.maybeExtractMemory(context.Background(), r.ID, "alice", "记住我喜欢绿茶", "好的，已记下")

	if *calls != 1 {
		t.Fatalf("degraded verdict must still extract, calls=%d want 1", *calls)
	}
	evs, _ := eng.Store.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == EventMemoryExtractSkipped {
			t.Fatalf("degraded Yes must not emit skipped; evs=%+v", evs)
		}
	}
	list, _ := eng.Memory.List("alice", 20, 0)
	if len(list) != 1 {
		t.Fatalf("expected one extracted memory, list=%+v", list)
	}
}

// AC-04 falsification: if the decision layer returns a raw error (a broken
// implementation, not the Chain), the engine must still extract — never let a
// decision error silently suppress a memory.
func TestDP1DeciderErrorStillExtracts(t *testing.T) {
	decider := &stubDecider{err: errors.New("decider exploded")}
	eng, r, calls := newDP1Engine(t, decider, decideSettings())

	eng.maybeExtractMemory(context.Background(), r.ID, "alice", "记住我喜欢绿茶", "好的")

	if *calls != 1 {
		t.Fatalf("decider error must fail open and still extract, calls=%d want 1", *calls)
	}
	evs, _ := eng.Store.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == EventMemoryExtractSkipped {
			t.Fatalf("decider error must not emit skipped; evs=%+v", evs)
		}
	}
}

// The probe context must be truncated so the gate call is never more expensive
// than the extraction it tries to save.
func TestDP1ProbeIsTruncated(t *testing.T) {
	decider := &stubDecider{ans: decide.Answer{Verdict: decide.VerdictNo}}
	eng, r, _ := newDP1Engine(t, decider, decideSettings())

	big := string([]rune(repeatRune('字', 4000)))
	eng.maybeExtractMemory(context.Background(), r.ID, "alice", big, big)

	if n := len([]rune(decider.got.Context)); n > decideProbeMaxRunes {
		t.Fatalf("probe context runes=%d must be <= %d", n, decideProbeMaxRunes)
	}
	if decider.got.Kind != decide.KindMemoryExtract {
		t.Fatalf("kind=%q want %q", decider.got.Kind, decide.KindMemoryExtract)
	}
	if decider.got.OnFail != decide.VerdictYes {
		t.Fatalf("OnFail=%v want VerdictYes", decider.got.OnFail)
	}
}

func repeatRune(r rune, n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return string(out)
}
