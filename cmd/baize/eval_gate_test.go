package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/eval"
)

func TestWriteEvalGateReport(t *testing.T) {
	res := eval.GateResult{
		Total: 2, Passed: 1, Failed: 1, PassRate: 0.5, MinPassRate: 0.9,
		Pass: false, RanAt: time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC),
		Failures: []eval.GateFailure{{
			ID: "ec_bad", Codes: []string{"BSR"}, Notes: "missing_reads:a|b",
		}},
	}
	path := filepath.Join(t.TempDir(), "gate.md")
	if err := writeEvalGateReport(path, res); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"# Eval gate report", "1/2 (50.0%)", "threshold: 90.0%", "failed", "ec_bad", `missing_reads:a\|b`} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
}

func TestGateOutcome(t *testing.T) {
	if got := gateOutcome(eval.GateResult{Empty: true}); got != "empty (no confirmed cases)" {
		t.Fatalf("empty=%q", got)
	}
	if got := gateOutcome(eval.GateResult{Pass: true}); got != "passed" {
		t.Fatalf("pass=%q", got)
	}
	if got := gateOutcome(eval.GateResult{}); got != "failed" {
		t.Fatalf("fail=%q", got)
	}
}
