package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/eval"
	"github.com/rebornace/baize/internal/store"
)

var errEvalGateFailed = errors.New("eval gate failed")

// runEvalGateCommand implements `baize eval-gate`: it opens the configured
// store directly (no HTTP server), scores every confirmed eval case, and exits
// non-zero when the pass rate is below the threshold or no cases exist.
func runEvalGateCommand(args []string) error {
	fs := flag.NewFlagSet("eval-gate", flag.ContinueOnError)
	cfgPath := fs.String("config", defaultConfigPath, "path to config yaml (for store driver/path)")
	minRate := fs.Float64("min", eval.GateDefaultMinRate, "minimum pass rate, 0..1 (default: stored setting or 0.9)")
	limit := fs.Int("limit", 0, "max confirmed cases to score (default 500, max 500)")
	jsonOut := fs.Bool("json", false, "print the result as JSON")
	reportPath := fs.String("report", "", "write a Markdown report to this path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *minRate < 0 || *minRate > 1 {
		return fmt.Errorf("min must be between 0 and 1")
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	st, err := store.OpenWithOptions(cfg.Store.Driver, store.OpenOptions{
		SQLitePath: cfg.Store.SQLitePath,
		DSN:        cfg.Store.DSN,
	})
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	if c, ok := st.(interface{ Close() error }); ok {
		defer c.Close()
	}

	min := *minRate
	if !flagWasSet(fs, "min") {
		if raw, ok, serr := st.GetSetting(eval.GateMinRateSettingKey); serr == nil && ok {
			var stored float64
			if json.Unmarshal(raw, &stored) == nil && stored >= 0 && stored <= 1 {
				min = stored
			}
		}
	}

	res, err := eval.RunGate(st, min, *limit)
	if err != nil {
		return fmt.Errorf("run gate: %w", err)
	}
	if *jsonOut {
		raw, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(raw))
	} else {
		printEvalGateSummary(res)
	}
	if *reportPath != "" {
		if err := writeEvalGateReport(*reportPath, res); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
	}
	if res.Empty || !res.Pass {
		return errEvalGateFailed
	}
	return nil
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func printEvalGateSummary(res eval.GateResult) {
	if res.Empty {
		fmt.Println("eval gate: no confirmed cases")
		return
	}
	fmt.Printf("eval gate: %d/%d passed (%.1f%%), threshold %.1f%%\n",
		res.Passed, res.Total, res.PassRate*100, res.MinPassRate*100)
	for _, f := range res.Failures {
		fmt.Printf("- %s %s %s\n", f.ID, strings.Join(f.Codes, ","), f.Notes)
	}
}

func writeEvalGateReport(path string, res eval.GateResult) error {
	var b strings.Builder
	b.WriteString("# Eval gate report\n\n")
	b.WriteString("- ran_at: " + res.RanAt.UTC().Format("2006-01-02T15:04:05Z") + "\n")
	fmt.Fprintf(&b, "- passed: %d/%d (%.1f%%)\n", res.Passed, res.Total, res.PassRate*100)
	fmt.Fprintf(&b, "- threshold: %.1f%%\n", res.MinPassRate*100)
	fmt.Fprintf(&b, "- result: %s\n", gateOutcome(res))
	b.WriteString("\n## Failures\n\n")
	if len(res.Failures) == 0 {
		b.WriteString("None.\n")
	} else {
		b.WriteString("| case | codes | notes |\n|------|-------|-------|\n")
		for _, f := range res.Failures {
			fmt.Fprintf(&b, "| %s | %s | %s |\n",
				f.ID, strings.Join(f.Codes, ","), strings.ReplaceAll(f.Notes, "|", "\\|"))
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func gateOutcome(res eval.GateResult) string {
	if res.Empty {
		return "empty (no confirmed cases)"
	}
	if res.Pass {
		return "passed"
	}
	return "failed"
}
