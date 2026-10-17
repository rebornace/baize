package eval_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/eval"
	"github.com/rebornace/baize/internal/store"
)

func upsertEvalCase(t *testing.T, st store.Store, c store.EvalCase) store.EvalCase {
	t.Helper()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = c.CreatedAt
	}
	if c.EventsJSON == nil {
		c.EventsJSON = []byte("[]")
	}
	if err := st.UpsertEvalCase(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func scoredConfirmed(id string, pass bool) store.EvalCase {
	now := time.Now().UTC()
	return store.EvalCase{
		ID:            id,
		Status:        store.EvalCaseConfirmed,
		Protocol:      "V0",
		Input:         "x",
		RequiredReads: []string{},
		Consequential: []string{},
		LastScorePass: &pass,
		LastScoredAt:  &now,
	}
}

func TestSetCalibrationRequiresScore(t *testing.T) {
	st := store.NewMemory()
	draft := upsertEvalCase(t, st, store.EvalCase{
		ID:     "ec_draft",
		Status: store.EvalCaseDraft,
		Input:  "x",
	})
	_, err := eval.SetCalibration(st, draft.ID, boolPtr(true), "")
	if err != eval.ErrNotCalibratable {
		t.Fatalf("draft: want ErrNotCalibratable, got %v", err)
	}

	unscored := upsertEvalCase(t, st, store.EvalCase{
		ID:            "ec_unscored",
		Status:        store.EvalCaseConfirmed,
		Protocol:      "V0",
		Input:         "x",
		RequiredReads: []string{},
		Consequential: []string{},
	})
	_, err = eval.SetCalibration(st, unscored.ID, boolPtr(true), "")
	if err != eval.ErrNotCalibratable {
		t.Fatalf("unscored: want ErrNotCalibratable, got %v", err)
	}
}

func TestSetCalibrationAgreeAndClear(t *testing.T) {
	st := store.NewMemory()
	c := upsertEvalCase(t, st, scoredConfirmed("ec1", true))

	got, err := eval.SetCalibration(st, c.ID, boolPtr(true), "  ok  ")
	if err != nil {
		t.Fatal(err)
	}
	if got.HumanAgree == nil || !*got.HumanAgree || got.HumanNote != "ok" || got.CalibratedAt == nil {
		t.Fatalf("agree true: %+v", got)
	}

	got, err = eval.SetCalibration(st, c.ID, boolPtr(false), "nope")
	if err != nil {
		t.Fatal(err)
	}
	if got.HumanAgree == nil || *got.HumanAgree || got.HumanNote != "nope" {
		t.Fatalf("agree false: %+v", got)
	}

	got, err = eval.SetCalibration(st, c.ID, nil, "ignored")
	if err != nil {
		t.Fatal(err)
	}
	if got.HumanAgree != nil || got.HumanNote != "" || got.CalibratedAt != nil {
		t.Fatalf("clear: %+v", got)
	}
}

func TestSetCalibrationNoteTooLong(t *testing.T) {
	st := store.NewMemory()
	c := upsertEvalCase(t, st, scoredConfirmed("ec1", true))
	note := strings.Repeat("日", 501)
	_, err := eval.SetCalibration(st, c.ID, boolPtr(true), note)
	if err != eval.ErrNoteTooLong {
		t.Fatalf("want ErrNoteTooLong, got %v", err)
	}
	got, _ := st.GetEvalCase(c.ID)
	if got.HumanAgree != nil {
		t.Fatalf("must not persist on error: %+v", got)
	}
}

func TestCalibrationSummaryRate(t *testing.T) {
	st := store.NewMemory()
	agree := true
	disagree := false
	cal := time.Now().UTC()
	pass := true
	scoredAt := time.Now().UTC()

	upsertEvalCase(t, st, store.EvalCase{
		ID: "ec_agree", Status: store.EvalCaseConfirmed, Protocol: "V0", Input: "a",
		RequiredReads: []string{}, Consequential: []string{},
		LastScorePass: &pass, LastScoredAt: &scoredAt,
		HumanAgree: &agree, CalibratedAt: &cal,
	})
	fail := false
	upsertEvalCase(t, st, store.EvalCase{
		ID: "ec_disagree", Status: store.EvalCaseConfirmed, Protocol: "V0", Input: "b",
		RequiredReads: []string{}, Consequential: []string{},
		LastScorePass: &fail, LastScoredAt: &scoredAt,
		HumanAgree: &disagree, CalibratedAt: &cal,
	})
	upsertEvalCase(t, st, store.EvalCase{
		ID: "ec_unscored", Status: store.EvalCaseConfirmed, Protocol: "V0", Input: "c",
		RequiredReads: []string{}, Consequential: []string{},
	})
	upsertEvalCase(t, st, store.EvalCase{
		ID: "ec_draft", Status: store.EvalCaseDraft, Input: "d",
	})

	sum, err := eval.CalibrationSummary(st)
	if err != nil {
		t.Fatal(err)
	}
	if sum.ScoredCount != 2 || sum.CalibratedCount != 2 || sum.AgreeCount != 1 || sum.DisagreeCount != 1 {
		t.Fatalf("counts: %+v", sum)
	}
	if sum.UncalibratedScoredCount != 0 {
		t.Fatalf("uncalibrated: %+v", sum)
	}
	if sum.AgreementRate == nil || *sum.AgreementRate != 0.5 {
		t.Fatalf("rate: %+v", sum)
	}
}

func TestCalibrationSummaryNoCalibrationsNullRate(t *testing.T) {
	st := store.NewMemory()
	pass := true
	scoredAt := time.Now().UTC()
	upsertEvalCase(t, st, store.EvalCase{
		ID: "ec1", Status: store.EvalCaseConfirmed, Protocol: "V0", Input: "a",
		RequiredReads: []string{}, Consequential: []string{},
		LastScorePass: &pass, LastScoredAt: &scoredAt,
	})

	sum, err := eval.CalibrationSummary(st)
	if err != nil {
		t.Fatal(err)
	}
	if sum.ScoredCount != 1 || sum.CalibratedCount != 0 || sum.AgreementRate != nil {
		t.Fatalf("want null rate: %+v", sum)
	}
	if sum.UncalibratedScoredCount != 1 {
		t.Fatalf("uncalibrated: %+v", sum)
	}
}

func boolPtr(b bool) *bool {
	return &b
}
