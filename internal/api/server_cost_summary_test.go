package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

func TestCostSummaryAndBudget(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "m", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	runRec, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: "c1", ModelProfileID: profile.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(runRec.ID, store.Event{
		Type: run.EventLLMUsage,
		Data: map[string]any{
			"model_profile_id": profile.ID, "prompt_tokens": 1000, "completion_tokens": 1000, "total_tokens": 2000,
		},
	})
	put := httptest.NewRequest(http.MethodPut, "/v0/settings/cost-budget",
		bytes.NewBufferString(`{"budget_usd_per_day":0.000001,"enforce":true}`))
	put.Header.Set("Authorization", "Bearer adm")
	putRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putRR, put)
	if putRR.Code != http.StatusOK {
		t.Fatalf("budget put=%d %s", putRR.Code, putRR.Body.String())
	}
	get := httptest.NewRequest(http.MethodGet, "/v0/settings/cost-summary?days=7", nil)
	get.Header.Set("Authorization", "Bearer adm")
	getRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRR, get)
	if getRR.Code != http.StatusOK {
		t.Fatalf("summary=%d %s", getRR.Code, getRR.Body.String())
	}
	var body struct {
		TodayTotal float64 `json:"today_total_usd"`
		Budget     float64 `json:"budget_usd_per_day"`
		Enforce    bool    `json:"budget_enforce"`
		Over       bool    `json:"over_budget"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TodayTotal <= 0 || body.Budget != 0.000001 || !body.Over || !body.Enforce {
		t.Fatalf("summary=%+v", body)
	}
	if err := srv.requireCostBudget(); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("requireCostBudget=%v", err)
	}
}

func TestCostSummaryReadsFinalizedDayRollup(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	if err := srv.Store.PutCostDay(store.CostDay{
		Day: yesterday, CostUSD: 0.42, Runs: 2, Finalized: true,
	}); err != nil {
		t.Fatal(err)
	}
	// No runs exist at all: the only way yesterday can report 0.42 is the rollup.
	get := httptest.NewRequest(http.MethodGet, "/v0/settings/cost-summary?days=3", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("summary=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Days []struct {
			Date     string  `json:"date"`
			TotalUSD float64 `json:"total_usd"`
			Runs     int     `json:"runs"`
		} `json:"days"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, d := range body.Days {
		if d.Date != yesterday {
			continue
		}
		if d.TotalUSD != 0.42 || d.Runs != 2 {
			t.Fatalf("yesterday=%+v", d)
		}
		return
	}
	t.Fatalf("yesterday missing from %+v", body.Days)
}

func TestRollupDayFinalizesOnlyPastCompleteDays(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	runs, _, final, err := srv.rollupDay(todayStart)
	if err != nil || final || runs != 0 {
		t.Fatalf("empty today runs=%v final=%v err=%v", runs, final, err)
	}
	if _, _, final, err := srv.rollupDay(todayStart.AddDate(0, 0, -1)); err != nil || !final {
		t.Fatalf("past day should finalize: final=%v err=%v", final, err)
	}

	runRec, err := srv.Store.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x", ConversationID: "c-rollup"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := srv.rollupDay(todayStart.AddDate(0, 0, -1)); err != nil {
		t.Fatal(err)
	}
	if err := srv.Store.UpdateRun(runRec.ID, store.StatusRunning, "", ""); err != nil {
		t.Fatal(err)
	}
	// Today never finalizes, even once every run is terminal.
	if err := srv.Store.UpdateRun(runRec.ID, store.StatusSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, final, err := srv.rollupDay(todayStart); err != nil || final {
		t.Fatalf("today final=%v err=%v", final, err)
	}
}
