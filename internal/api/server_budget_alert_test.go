package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

func TestBudgetAlertStateTransition(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "alert-model", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	runRec, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: "c-alert", ModelProfileID: profile.ID,
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
	if err := st.UpsertSetting(costBudgetSettingKey, []byte("0.005")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(costBudgetMonthKey, []byte("100")); err != nil {
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
	if state.DayLevel != "exceeded" || state.MonthLevel != "" {
		t.Fatalf("state=%+v", state)
	}
	if state.DayKey == "" || state.MonthKey == "" {
		t.Fatalf("period keys missing: %+v", state)
	}
	// A second check keeps the state (no duplicate alert attempt).
	if _, err := srv.checkBudgetAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw2, _, _ := st.GetSetting(costAlertStateKey)
	var state2 budgetAlertState
	_ = json.Unmarshal(raw2, &state2)
	if state2 != state {
		t.Fatalf("state changed on repeat: %+v -> %+v", state, state2)
	}
}

func TestAlertRank(t *testing.T) {
	if alertRank("") != 0 || alertRank("warning") != 1 || alertRank("exceeded") != 2 {
		t.Fatal("alertRank ordering changed")
	}
}
