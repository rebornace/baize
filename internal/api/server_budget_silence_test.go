package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func TestBudgetAlertSilenceAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()

	until := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	put := httptest.NewRequest(http.MethodPut, "/v0/settings/budget-alert-silence",
		bytes.NewBufferString(`{"until":"`+until+`","reason":"maintenance"}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/v0/settings/budget-alert-silence", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d", rr.Code)
	}
	var body struct {
		Until  string `json:"until"`
		Reason string `json:"reason"`
		Active bool   `json:"active"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Active || body.Reason != "maintenance" || body.Until == "" {
		t.Fatalf("body=%+v", body)
	}
	if !srv.budgetAlertsSilenced(time.Now().UTC()) {
		t.Fatal("silence should be active")
	}

	// Empty until clears the window.
	put = httptest.NewRequest(http.MethodPut, "/v0/settings/budget-alert-silence",
		bytes.NewBufferString(`{"until":""}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("clear=%d %s", rr.Code, rr.Body.String())
	}
	if srv.budgetAlertsSilenced(time.Now().UTC()) {
		t.Fatal("silence should be cleared")
	}
}

func TestBudgetAlertSilenceRejectsPast(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	until := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodPut, "/v0/settings/budget-alert-silence",
		bytes.NewBufferString(`{"until":"`+until+`"}`))
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestSilenceSuppressesBudgetAlertsWithoutAdvancingState(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "silence", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	runRec, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: "c-silence", ModelProfileID: profile.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(runRec.ID, store.Event{
		Type: "llm.usage",
		Data: map[string]any{
			"model_profile_id": profile.ID, "prompt_tokens": 1000, "completion_tokens": 1000, "total_tokens": 2000,
		},
	})
	if err := st.UpsertSetting(costBudgetMonthKey, []byte("0.005")); err != nil {
		t.Fatal(err)
	}

	raw, _ := json.Marshal(costAlertSilence{Until: time.Now().UTC().Add(time.Hour), Reason: "window"})
	if err := st.UpsertSetting(costAlertSilenceKey, raw); err != nil {
		t.Fatal(err)
	}
	if n, err := srv.checkBudgetAlerts(t.Context()); err != nil || n != 0 {
		t.Fatalf("sent=%d err=%v", n, err)
	}
	if _, ok, _ := st.GetSetting(costAlertHistoryKey); ok {
		t.Fatal("history should stay empty during silence")
	}
	if _, ok, _ := st.GetSetting(costAlertStateKey); ok {
		t.Fatal("dedupe state must not advance during silence")
	}
}
