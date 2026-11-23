package pricing

import (
	"math"
	"testing"
)

func TestLookupLongestPrefixAndCost(t *testing.T) {
	table := Table{Currency: "USD", Entries: []Entry{
		{Model: "gpt-4o", InputPerMTok: 2.5, OutputPerMTok: 10, CachedInputPerMTok: 1.25},
		{Model: "gpt-4o-mini", InputPerMTok: 0.15, OutputPerMTok: 0.6, CachedInputPerMTok: 0.075},
	}}
	e, ok := table.Lookup("GPT-4o-mini-2024-07-18")
	if !ok || e.Model != "gpt-4o-mini" {
		t.Fatalf("lookup=%+v ok=%v", e, ok)
	}
	cost, _, ok := table.Cost("gpt-4o-2024-08-06", 1_000_000, 1_000_000, 0)
	if !ok || math.Abs(cost-12.5) > 1e-9 {
		t.Fatalf("cost=%v ok=%v", cost, ok)
	}
	cached, _, _ := table.Cost("gpt-4o", 1_000_000, 0, 1_000_000)
	if math.Abs(cached-1.25) > 1e-9 {
		t.Fatalf("cached cost=%v", cached)
	}
}

func TestValidateRejectsBadEntries(t *testing.T) {
	if err := (Table{Entries: []Entry{{Model: "x", InputPerMTok: -1}}}).Validate(); err == nil {
		t.Fatal("want negative error")
	}
	if err := (Table{Entries: []Entry{{Model: "x"}, {Model: "X"}}}).Validate(); err == nil {
		t.Fatal("want duplicate error")
	}
}
