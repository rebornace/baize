package channel

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestParseHITLDecision(t *testing.T) {
	cases := []struct {
		in      string
		approve bool
		ok      bool
	}{
		{"批准", true, true},
		{"  同意  ", true, true},
		{"approve", true, true},
		{"YES", true, true},
		{"拒绝", false, true},
		{"reject!", false, true},
		{"请稍候", false, false},
		{"批准一下吧", false, false},
		{"", false, false},
	}
	for _, tc := range cases {
		approve, ok := ParseHITLDecision(tc.in)
		if approve != tc.approve || ok != tc.ok {
			t.Fatalf("ParseHITLDecision(%q)=(%v,%v) want (%v,%v)", tc.in, approve, ok, tc.approve, tc.ok)
		}
	}
}

func TestFormatHITLNotify(t *testing.T) {
	got := FormatHITLNotify(&store.HITLPayload{
		ToolName: "create_ticket",
		Prompt:   "确认开单？",
	})
	if !strings.Contains(got, "create_ticket") || !strings.Contains(got, "确认开单？") {
		t.Fatalf("notify=%q", got)
	}
	if !strings.Contains(got, "批准") || !strings.Contains(got, "拒绝") {
		t.Fatalf("missing decision hints: %q", got)
	}
}
