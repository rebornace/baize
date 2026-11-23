package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

func TestRunBindingIncludesTokenUsage(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "m", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	mini, err := st.UpsertModelProfile(store.ModelProfile{Name: "mini", Model: "gpt-4o-mini"})
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
		Type: run.EventRunBinding,
		Data: map[string]any{"model_profile_id": "mp1"},
	})
	_ = st.AppendEvent(r.ID, store.Event{
		Type: run.EventLLMUsage,
		Data: map[string]any{
			"model_profile_id": mini.ID,
			"prompt_tokens":    10, "completion_tokens": 5, "total_tokens": 15, "cached_tokens": 2,
		},
	})
	_ = st.AppendEvent(r.ID, store.Event{
		Type: run.EventLLMUsage,
		Data: map[string]any{
			"prompt_tokens": 20, "completion_tokens": 10, "total_tokens": 30,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+r.ID+"/binding", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Usage struct {
			Turns            int     `json:"turns"`
			PromptTokens     int     `json:"prompt_tokens"`
			CompletionTokens int     `json:"completion_tokens"`
			TotalTokens      int     `json:"total_tokens"`
			CachedTokens     int     `json:"cached_tokens"`
			Priced           bool    `json:"priced"`
			CostUSD          float64 `json:"cost_usd"`
			Currency         string  `json:"currency"`
			PricedTurns      int     `json:"priced_turns"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	u := body.Usage
	if u.Turns != 2 || u.PromptTokens != 30 || u.CompletionTokens != 15 || u.TotalTokens != 45 || u.CachedTokens != 2 {
		t.Fatalf("usage=%+v", u)
	}
	if !u.Priced || u.Currency != "USD" || u.PricedTurns != 2 {
		t.Fatalf("cost=%+v", u)
	}
	if math.Abs(u.CostUSD-0.000154) > 1e-9 {
		t.Fatalf("cost_usd=%v want 0.000154", u.CostUSD)
	}
}
