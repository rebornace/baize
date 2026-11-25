package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

func TestModelCostBudgetsAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	profile, err := srv.Store.UpsertModelProfile(store.ModelProfile{
		Name: "model-budget", Model: "gpt-4o",
	})
	if err != nil {
		t.Fatal(err)
	}
	runRec, err := srv.Store.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: "c-model-budget", ModelProfileID: profile.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = srv.Store.AppendEvent(runRec.ID, store.Event{
		Type: run.EventLLMUsage,
		Data: map[string]any{
			"model_profile_id": profile.ID, "prompt_tokens": 1000, "completion_tokens": 1000, "total_tokens": 2000,
		},
	})
	h := srv.Handler()

	put := httptest.NewRequest(http.MethodPut, "/v0/settings/model-cost-budgets",
		bytes.NewBufferString(`{"model_profile_id":"`+profile.ID+
			`","budget_usd_per_day":1,"budget_usd_per_month":20,"warning_ratio":0.5,"exceeded_ratio":1.2}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	var saved modelBudgetJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Name != "model-budget" || saved.BudgetDay != 1 || saved.SpentDay <= 0 || saved.SpentMonth <= 0 {
		t.Fatalf("saved=%+v", saved)
	}
	if d, m := srv.modelBudgets(profile.ID); d != 1 || m != 20 {
		t.Fatalf("budgets=%v/%v", d, m)
	}

	get := httptest.NewRequest(http.MethodGet, "/v0/settings/model-cost-budgets", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Items []modelBudgetJSON `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range body.Items {
		if item.ModelProfileID != profile.ID {
			continue
		}
		found = true
		if item.WarningRatio != 0.5 || item.ExceededRatio != 1.2 || item.SpentDay <= 0 {
			t.Fatalf("item=%+v", item)
		}
	}
	if !found {
		t.Fatalf("profile missing from %+v", body.Items)
	}

	badThreshold := httptest.NewRequest(http.MethodPut, "/v0/settings/model-cost-budgets",
		bytes.NewBufferString(`{"model_profile_id":"`+profile.ID+
			`","budget_usd_per_day":1,"warning_ratio":1.2,"exceeded_ratio":0.5}`))
	badThreshold.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, badThreshold)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad thresholds=%d", rr.Code)
	}

	missing := httptest.NewRequest(http.MethodPut, "/v0/settings/model-cost-budgets",
		bytes.NewBufferString(`{"model_profile_id":"nope","budget_usd_per_day":1}`))
	missing.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, missing)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing profile=%d", rr.Code)
	}
}

func TestModelBudgetAlertState(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "model-alert", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	runRec, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: "c-model-alert", ModelProfileID: profile.ID,
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
	if err := st.UpsertSetting(modelBudgetDayPrefix+profile.ID, []byte("0.005")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(modelBudgetMonthPrefix+profile.ID, []byte("100")); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.checkBudgetAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, ok, err := st.GetSetting(costAlertStateKey)
	if err != nil || !ok {
		t.Fatalf("state missing ok=%v err=%v", ok, err)
	}
	var state budgetAlertState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if got := state.Scopes["model:"+profile.ID]; got.DayLevel != "exceeded" {
		t.Fatalf("model scope=%+v all=%+v", got, state.Scopes)
	}
}
