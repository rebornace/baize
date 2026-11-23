package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: "c1", ModelProfileID: profile.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(r.ID, store.Event{
		Type: run.EventLLMUsage,
		Data: map[string]any{"model_profile_id": profile.ID, "prompt_tokens": 1000, "completion_tokens": 1000, "total_tokens": 2000},
	})
	put := httptest.NewRequest(http.MethodPut, "/v0/settings/cost-budget",
		bytes.NewBufferString(`{"budget_usd_per_day":0.000001}`))
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
		Over       bool    `json:"over_budget"`
		Days       []struct {
			Runs int `json:"runs"`
		} `json:"days"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TodayTotal <= 0 || body.Budget != 0.000001 || !body.Over {
		t.Fatalf("summary=%+v", body)
	}
}
