package api

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/webhook"
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
	if !reflect.DeepEqual(state2, state) {
		t.Fatalf("state changed on repeat: %+v -> %+v", state, state2)
	}
}

func TestScopeThresholdOverride(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "scope-threshold", Model: "gpt-4o"})
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
	for key, val := range map[string]string{
		costWarnRatioKey:               "0.8",
		costExceededRatioKey:           "1",
		agentBudgetDayPrefix + "a1":    "0.05",
		agentBudgetMonthPrefix + "a1":  "100",
		agentBudgetWarnPrefix + "a1":   "0.1",
		agentBudgetExceedPrefix + "a1": "0.2",
	} {
		if err := st.UpsertSetting(key, []byte(val)); err != nil {
			t.Fatal(err)
		}
	}
	clearAgentCostCache()
	srv.Webhook = webhook.NewDispatcher(st, webhook.Config{URL: "http://events.invalid/hook"})
	sent, err := srv.checkBudgetAlerts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sent != 1 {
		t.Fatalf("sent=%d want 1 (scope threshold must fire below global warning)", sent)
	}
	entries, err := st.ListWebhookOutbox(nil, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("outbox=%d err=%v", len(entries), err)
	}
	var payload struct {
		Event struct {
			Data struct {
				Target   string `json:"target"`
				TargetID string `json:"target_id"`
				Level    string `json:"level"`
			} `json:"data"`
		} `json:"event"`
	}
	if err := json.Unmarshal(entries[0].PayloadJSON, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Event.Data.Target != "agent" || payload.Event.Data.TargetID != "a1" ||
		payload.Event.Data.Level != "exceeded" {
		t.Fatalf("payload=%+v", payload.Event.Data)
	}
}

func TestAlertRank(t *testing.T) {
	if alertRank("") != 0 || alertRank("warning") != 1 || alertRank("exceeded") != 2 {
		t.Fatal("alertRank ordering changed")
	}
}

func TestScopedBudgetAlerts(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "scoped-alert", Model: "gpt-4o"})
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
	for key, val := range map[string]string{
		agentBudgetDayPrefix + "a1":          "0.005",
		agentBudgetMonthPrefix + "a1":        "100",
		conversationBudgetDayPrefix + "c1":   "0.005",
		conversationBudgetMonthPrefix + "c1": "100",
	} {
		if err := st.UpsertSetting(key, []byte(val)); err != nil {
			t.Fatal(err)
		}
	}
	clearAgentCostCache()
	clearConversationCostCache()
	srv.Webhook = webhook.NewDispatcher(st, webhook.Config{URL: "http://events.invalid/hook"})

	sent, err := srv.checkBudgetAlerts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sent != 2 {
		t.Fatalf("sent=%d, want 2", sent)
	}
	entries, err := st.ListWebhookOutbox(nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("outbox=%d, want 2", len(entries))
	}
	type alertPayload struct {
		Event struct {
			Type string `json:"type"`
			Data struct {
				Scope    string `json:"scope"`
				Level    string `json:"level"`
				Period   string `json:"period"`
				Target   string `json:"target"`
				TargetID string `json:"target_id"`
			} `json:"data"`
		} `json:"event"`
	}
	got := map[string]bool{}
	for _, entry := range entries {
		var p alertPayload
		if err := json.Unmarshal(entry.PayloadJSON, &p); err != nil {
			t.Fatal(err)
		}
		if p.Event.Type != "budget.alert" {
			t.Fatalf("type=%q", p.Event.Type)
		}
		got[p.Event.Data.Target+"/"+p.Event.Data.TargetID] = p.Event.Data.Level == "exceeded" &&
			p.Event.Data.Scope == "day" && p.Event.Data.Period == dayKeyNow()
	}
	if !got["agent/a1"] || !got["conversation/c1"] {
		t.Fatalf("alerts=%+v", got)
	}
	stateRaw, _, _ := st.GetSetting(costAlertStateKey)
	var state budgetAlertState
	if err := json.Unmarshal(stateRaw, &state); err != nil {
		t.Fatal(err)
	}
	if state.Scopes["agent:a1"].DayLevel != "exceeded" || state.Scopes["conversation:c1"].DayLevel != "exceeded" {
		t.Fatalf("scope state=%+v", state.Scopes)
	}

	sent, err = srv.checkBudgetAlerts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sent != 0 {
		t.Fatalf("repeat sent=%d, want 0", sent)
	}
	after, _ := st.ListWebhookOutbox(nil, 50)
	if len(after) != len(entries) {
		t.Fatalf("outbox grew on repeat: %d -> %d", len(entries), len(after))
	}
}

func dayKeyNow() string {
	return time.Now().UTC().Format("2006-01-02")
}
