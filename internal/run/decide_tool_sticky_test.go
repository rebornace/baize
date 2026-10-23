package run

import (
	"testing"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/runtimecfg"
)

func TestNameSetJaccard(t *testing.T) {
	if g := nameSetJaccard([]string{"a", "b"}, []string{"a", "b"}); g != 1 {
		t.Fatalf("identical: %v", g)
	}
	if g := nameSetJaccard([]string{"a", "b"}, []string{"a", "c"}); g != 1.0/3.0 {
		t.Fatalf("overlap1: %v", g)
	}
	if g := nameSetJaccard([]string{"a"}, []string{"b"}); g != 0 {
		t.Fatalf("disjoint: %v", g)
	}
}

func TestApplyStickyNarrowReusesSimilarSet(t *testing.T) {
	eng := &Engine{
		Settings: fakeKnobs{k: runtimecfg.Knobs{
			DecideToolStickyEnabled:    true,
			DecideToolStickyMinJaccard: 0.5,
		}},
	}
	catalog := []llm.ToolSpec{
		{Name: "t1"}, {Name: "t2"}, {Name: "t3"}, {Name: "t4"},
	}
	first := []llm.ToolSpec{{Name: "t1"}, {Name: "t2"}, {Name: "t3"}}
	sent, reused, _ := eng.applyStickyNarrow("c1", first, catalog)
	if reused || len(sent) != 3 {
		t.Fatalf("first: reused=%v sent=%v", reused, toolNames(sent))
	}
	// One swap: Jaccard = 2/4 = 0.5 → reuse at threshold 0.5
	second := []llm.ToolSpec{{Name: "t1"}, {Name: "t2"}, {Name: "t4"}}
	sent, reused, j := eng.applyStickyNarrow("c1", second, catalog)
	if !reused {
		t.Fatalf("expected sticky reuse, j=%v sent=%v", j, toolNames(sent))
	}
	if got := sortedNames(sent); got[0] != "t1" || got[1] != "t2" || got[2] != "t3" {
		t.Fatalf("must keep previous set order/content: %v", toolNames(sent))
	}
}

func TestApplyStickyNarrowReplacesOnLargeChange(t *testing.T) {
	eng := &Engine{
		Settings: fakeKnobs{k: runtimecfg.Knobs{
			DecideToolStickyEnabled:    true,
			DecideToolStickyMinJaccard: 0.7,
		}},
	}
	catalog := []llm.ToolSpec{
		{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"},
	}
	_, _, _ = eng.applyStickyNarrow("c1", []llm.ToolSpec{{Name: "a"}, {Name: "b"}}, catalog)
	sent, reused, j := eng.applyStickyNarrow("c1", []llm.ToolSpec{{Name: "c"}, {Name: "d"}, {Name: "e"}}, catalog)
	if reused {
		t.Fatalf("large change must not reuse (j=%v)", j)
	}
	if got := sortedNames(sent); len(got) != 3 || got[0] != "c" {
		t.Fatalf("sent=%v", toolNames(sent))
	}
}

func TestApplyStickyNarrowDisabled(t *testing.T) {
	eng := &Engine{
		Settings: fakeKnobs{k: runtimecfg.Knobs{DecideToolStickyEnabled: false}},
	}
	catalog := []llm.ToolSpec{{Name: "a"}, {Name: "b"}}
	_, _, _ = eng.applyStickyNarrow("c1", []llm.ToolSpec{{Name: "a"}}, catalog)
	sent, reused, _ := eng.applyStickyNarrow("c1", []llm.ToolSpec{{Name: "b"}}, catalog)
	if reused || sent[0].Name != "b" {
		t.Fatalf("disabled sticky must pass through: reused=%v sent=%v", reused, toolNames(sent))
	}
}
