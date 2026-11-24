package eval

import (
	"time"

	"github.com/rebornace/baize/internal/store"
)

// GateMinRateSettingKey is the settings key holding the UI gate threshold.
const GateMinRateSettingKey = "eval_gate_min_pass_rate"

// GateDefaultMinRate is used when no threshold is stored.
const GateDefaultMinRate = 0.9

const gateCaseLimit = 500
const gateMaxFailures = 50

// GateFailure is one case that did not pass the regression gate.
type GateFailure struct {
	ID    string   `json:"id"`
	Input string   `json:"input"`
	Codes []string `json:"codes"`
	Notes string   `json:"notes"`
}

// GateResult is the deterministic regression-gate outcome.
type GateResult struct {
	Total       int           `json:"total"`
	Passed      int           `json:"passed"`
	Failed      int           `json:"failed"`
	PassRate    float64       `json:"pass_rate"`
	MinPassRate float64       `json:"min_pass_rate"`
	Pass        bool          `json:"pass"`
	Empty       bool          `json:"empty"`
	RanAt       time.Time     `json:"ran_at"`
	Failures    []GateFailure `json:"failures"`
}

// RunGate scores every confirmed eval case and compares the pass rate with
// minRate. It never calls a model.
func RunGate(st store.Store, minRate float64, limit int) (GateResult, error) {
	if limit <= 0 || limit > gateCaseLimit {
		limit = gateCaseLimit
	}
	cases, err := st.ListEvalCases(limit)
	if err != nil {
		return GateResult{}, err
	}
	res := GateResult{
		MinPassRate: minRate,
		RanAt:       time.Now().UTC(),
		Failures:    []GateFailure{},
	}
	for _, c := range cases {
		if c.Status != store.EvalCaseConfirmed {
			continue
		}
		res.Total++
		out, serr := ScoreCase(st, c.ID)
		if serr != nil {
			res.Failed++
			if len(res.Failures) < gateMaxFailures {
				res.Failures = append(res.Failures, GateFailure{
					ID: c.ID, Input: previewRunes(c.Input, 120),
					Codes: []string{"SCORE_ERROR"}, Notes: serr.Error(),
				})
			}
			continue
		}
		if out.Pass {
			res.Passed++
			continue
		}
		res.Failed++
		if len(res.Failures) < gateMaxFailures {
			codes := out.Codes
			if codes == nil {
				codes = []string{}
			}
			res.Failures = append(res.Failures, GateFailure{
				ID: c.ID, Input: previewRunes(c.Input, 120), Codes: codes, Notes: out.Notes,
			})
		}
	}
	res.Empty = res.Total == 0
	if res.Total > 0 {
		res.PassRate = roundGate(float64(res.Passed) / float64(res.Total))
		res.Pass = res.PassRate >= minRate
	}
	return res, nil
}

func previewRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func roundGate(v float64) float64 {
	return float64(int64(v*1e4+0.5)) / 1e4
}
