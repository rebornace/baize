package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

func TestConversationCostBudgetAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	get := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/cost-budget", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Day   float64 `json:"budget_usd_per_day"`
		Month float64 `json:"budget_usd_per_month"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body.Day != 0 || body.Month != 0 {
		t.Fatalf("default=%+v err=%v", body, err)
	}
	put := httptest.NewRequest(http.MethodPut, "/v0/conversations/c1/cost-budget",
		bytes.NewBufferString(`{"budget_usd_per_day":1,"budget_usd_per_month":20}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	if d, m := srv.conversationBudgets("c1"); d != 1 || m != 20 {
		t.Fatalf("budgets=%v/%v", d, m)
	}
	bad := httptest.NewRequest(http.MethodPut, "/v0/conversations/c1/cost-budget",
		bytes.NewBufferString(`{"budget_usd_per_day":-1}`))
	bad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad=%d", rr.Code)
	}
}

func TestConversationBudgetEnforcement(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	if err := st.UpsertSetting(costBudgetEnforceKey, []byte("true")); err != nil {
		t.Fatal(err)
	}
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "conv-budget", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	runRec, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a1", Input: "x", ConversationID: "c1", ModelProfileID: profile.ID,
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
	if err := st.UpsertSetting(conversationBudgetDayPrefix+"c1", []byte("0.005")); err != nil {
		t.Fatal(err)
	}
	clearConversationCostCache()
	if err := srv.requireCostBudgetFor("", "c1"); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("err=%v, want ErrCostBudgetExceeded", err)
	}
	if err := srv.requireCostBudgetFor("", "c2"); err != nil {
		t.Fatalf("other conv err=%v", err)
	}
}
