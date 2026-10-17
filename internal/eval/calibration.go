package eval

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/store"
)

const calibrationSummaryListLimit = 500 // same cap as store.ListEvalCases (EV1a)

// Summary aggregates human calibration against code scores for calibratable cases.
type Summary struct {
	ScoredCount             int
	CalibratedCount         int
	AgreeCount              int
	DisagreeCount           int
	AgreementRate           *float64 // nil when CalibratedCount == 0
	UncalibratedScoredCount int
}

func isCalibratable(c store.EvalCase) bool {
	return c.Status == store.EvalCaseConfirmed &&
		c.LastScorePass != nil &&
		c.LastScoredAt != nil
}

func validateHumanNote(note string) (string, error) {
	trimmed := strings.TrimSpace(note)
	if utf8.RuneCountInString(trimmed) > 500 {
		return "", ErrNoteTooLong
	}
	return trimmed, nil
}

// SetCalibration writes or clears human agree/note on a scored confirmed case.
func SetCalibration(st store.Store, id string, agree *bool, note string) (store.EvalCase, error) {
	c, err := st.GetEvalCase(id)
	if errors.Is(err, store.ErrEvalCaseNotFound) {
		return store.EvalCase{}, ErrCaseNotFound
	}
	if err != nil {
		return store.EvalCase{}, err
	}
	if !isCalibratable(c) {
		return store.EvalCase{}, ErrNotCalibratable
	}

	now := time.Now().UTC()
	if agree == nil {
		c.HumanAgree = nil
		c.HumanNote = ""
		c.CalibratedAt = nil
	} else {
		trimmed, err := validateHumanNote(note)
		if err != nil {
			return store.EvalCase{}, err
		}
		v := *agree
		c.HumanAgree = &v
		c.HumanNote = trimmed
		t := now
		c.CalibratedAt = &t
	}
	c.UpdatedAt = now
	if err := st.UpsertEvalCase(c); err != nil {
		return store.EvalCase{}, err
	}
	return st.GetEvalCase(c.ID)
}

// CalibrationSummary aggregates calibration stats over ListEvalCases (max 500 rows).
func CalibrationSummary(st store.Store) (Summary, error) {
	cases, err := st.ListEvalCases(calibrationSummaryListLimit)
	if err != nil {
		return Summary{}, err
	}
	var sum Summary
	for _, c := range cases {
		if !isCalibratable(c) {
			continue
		}
		sum.ScoredCount++
		if c.HumanAgree == nil {
			continue
		}
		sum.CalibratedCount++
		if *c.HumanAgree {
			sum.AgreeCount++
		} else {
			sum.DisagreeCount++
		}
	}
	sum.UncalibratedScoredCount = sum.ScoredCount - sum.CalibratedCount
	if sum.CalibratedCount > 0 {
		rate := float64(sum.AgreeCount) / float64(sum.CalibratedCount)
		sum.AgreementRate = &rate
	}
	return sum, nil
}
