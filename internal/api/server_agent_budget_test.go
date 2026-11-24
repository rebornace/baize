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

func TestAgentCostBudgetsAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	srv.Store.UpsertAgent(store.Agent{ID: "a1", System: "s"})
	h := srv.Handler()
	put := httptest.NewRequest(http.MethodPut, "/v0/settings/agent-cost-budgets",
		bytes.NewBufferString(`{"agent_id":"a1","budget_usd_per_day":1,"budget_usd_per_month":20}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	if d, m := srv.agentBudgets("a1"); d != 1 || m != 20 {
		t.Fatalf("budgets=%v/%v", d, m)
	}
	get := httptest.NewRequest(http.MethodGet, "/v0/settings/agent-cost-budgets", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d", rr.Code)
	}
	var body struct {
		Items []struct {
			AgentID string  `json:"agent_id"`
			Day     float64 `json:"budget_usd_per_day"`
			Month   float64 `json:"budget_usd_per_month"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].AgentID != "a1" || body.Items[0].Day != 1 {
		t.Fatalf("items=%+v", body.Items)
	}
	bad := httptest.NewRequest(http.MethodPut, "/v0/settings/agent-cost-budgets",
		bytes.NewBufferString(`{"agent_id":"","budget_usd_per_day":1}`))
	bad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad=%d", rr.Code)
	}
}

func TestAgentBudgetEnforcement(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	if err := st.UpsertSetting(costBudgetEnforceKey, []byte("true")); err != nil {
		t.Fatal(err)
	}
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "agent-budget", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	runRec, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a1", Input: "x", ConversationID: "c-agent", ModelProfileID: profile.ID,
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
	if err := st.UpsertSetting(agentBudgetDayPrefix+"a1", []byte("0.005")); err != nil {
		t.Fatal(err)
	}
	clearAgentCostCache()
	if err := srv.requireCostBudgetFor("a1", ""); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("err=%v, want ErrCostBudgetExceeded", err)
	}
	if err := srv.requireCostBudgetFor("other", ""); err != nil {
		t.Fatalf("other agent err=%v", err)
	}
}

func TestCostAlertThresholdsConfigurable(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	put := httptest.NewRequest(http.MethodPut, "/v0/settings/cost-budget",
		bytes.NewBufferString(`{"budget_usd_per_day":1,"budget_usd_per_month":10,"warning_ratio":0.5,"exceeded_ratio":2,"enforce":false}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	if warn, exceeded := srv.costAlertThresholds(); warn != 0.5 || exceeded != 2 {
		t.Fatalf("thresholds=%v/%v", warn, exceeded)
	}
	bad := httptest.NewRequest(http.MethodPut, "/v0/settings/cost-budget",
		bytes.NewBufferString(`{"budget_usd_per_day":1,"warning_ratio":2,"exceeded_ratio":1,"enforce":false}`))
	bad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad=%d", rr.Code)
	}
}

func TestAgentCostTotalsCached(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "agent-total", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	runRec, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a1", Input: "x", ConversationID: "c-total", ModelProfileID: profile.ID,
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
	clearAgentCostCache()
	day, month, err := srv.agentCostTotals("a1", time.Now().UTC())
	if err != nil || day <= 0 || month < day {
		t.Fatalf("day=%v month=%v err=%v", day, month, err)
	}
}
