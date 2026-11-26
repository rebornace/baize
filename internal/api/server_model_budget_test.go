package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

func postRunRaw(t *testing.T, srv *Server, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/runs", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

func postRunBody(t *testing.T, srv *Server, body map[string]any) *store.Run {
	t.Helper()
	rr := postRunRaw(t, srv, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("post run=%d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	rec, err := srv.Store.GetRun(resp.RunID)
	if err != nil || rec == nil {
		t.Fatalf("get run %q: %v", resp.RunID, err)
	}
	return rec
}

func decodeErrorBody(t *testing.T, body []byte) (code, message string) {
	t.Helper()
	var wrap struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		t.Fatalf("decode error: %v body=%s", err, body)
	}
	return wrap.Error.Code, wrap.Error.Message
}

func TestModelBudgetRoutingAvoidance(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	st.UpsertAgent(store.Agent{ID: "a1", System: "s"})
	premium, err := st.UpsertModelProfile(store.ModelProfile{
		Name: "premium", Model: "gpt-4o", AutoTier: store.AutoTierStandard,
	})
	if err != nil {
		t.Fatal(err)
	}
	cheap, err := st.UpsertModelProfile(store.ModelProfile{
		Name: "cheap", Model: "gpt-4o-mini", AutoTier: store.AutoTierStandard,
	})
	if err != nil {
		t.Fatal(err)
	}
	seedCost := func(profileID, conv string) {
		t.Helper()
		r, err := st.CreateRun(store.CreateRunInput{
			AgentID: "a1", Input: "seed", ConversationID: conv,
			ModelProfileID: profileID, CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r.ID, store.StatusSucceeded, "ok", ""); err != nil {
			t.Fatal(err)
		}
		if err := st.PutRunCost(store.RunCost{
			RunID: r.ID, Day: time.Now().UTC().Format("2006-01-02"), CostUSD: 1, Priced: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	seedCost(premium.ID, "c-premium-seed")
	if err := st.UpsertSetting(costBudgetEnforceKey, []byte("true")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(modelBudgetDayPrefix+premium.ID, []byte("0.005")); err != nil {
		t.Fatal(err)
	}

	run := postRunBody(t, srv, map[string]any{
		"agent_id": "a1", "input": "hi", "conversation_id": "c-auto",
	})
	if run.ModelProfileID != cheap.ID {
		t.Fatalf("auto routed to %q, want cheap %q", run.ModelProfileID, cheap.ID)
	}

	// All models exhausted: Auto has nowhere to go.
	seedCost(cheap.ID, "c-cheap-seed")
	if err := st.UpsertSetting(modelBudgetDayPrefix+cheap.ID, []byte("0.005")); err != nil {
		t.Fatal(err)
	}
	rr := postRunRaw(t, srv, map[string]any{
		"agent_id": "a1", "input": "hi", "conversation_id": "c-all-over",
	})
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("all over status=%d body=%s", rr.Code, rr.Body.String())
	}
	if code, _ := decodeErrorBody(t, rr.Body.Bytes()); code != "cost_budget_exceeded" {
		t.Fatalf("all over code=%q", code)
	}

	// Manual choice of an exhausted model is rejected, never rerouted.
	rr = postRunRaw(t, srv, map[string]any{
		"agent_id": "a1", "input": "hi", "conversation_id": "c-manual",
		"model_profile_id": premium.ID,
	})
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("manual over status=%d body=%s", rr.Code, rr.Body.String())
	}
}

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
