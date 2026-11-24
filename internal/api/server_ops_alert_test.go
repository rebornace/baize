package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func TestOpsAlertStateAndCooldown(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	for i := 0; i < 3; i++ {
		r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r.ID, store.StatusFailed, "", "boom"); err != nil {
			t.Fatal(err)
		}
	}
	waiting, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "wait", ConversationID: "c-wait"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(waiting.ID, store.StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}

	if _, err := srv.checkOpsAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, ok, err := st.GetSetting(opsAlertStateKey)
	if err != nil || !ok {
		t.Fatalf("state missing ok=%v err=%v", ok, err)
	}
	var state opsAlertState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if state.FailureAt == "" || state.WaitingAt == "" {
		t.Fatalf("state=%+v", state)
	}
	before := state
	if _, err := srv.checkOpsAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw2, _, _ := st.GetSetting(opsAlertStateKey)
	var state2 opsAlertState
	_ = json.Unmarshal(raw2, &state2)
	if state2 != before {
		t.Fatalf("state changed inside cooldown: %+v -> %+v", before, state2)
	}
}

func TestCooldownElapsed(t *testing.T) {
	now := time.Now().UTC()
	if !cooldownElapsed("", now, time.Minute) {
		t.Fatal("empty timestamp should be ready")
	}
	if cooldownElapsed(now.Add(-30*time.Second).Format(time.RFC3339), now, time.Minute) {
		t.Fatal("recent timestamp should be cooling down")
	}
	if !cooldownElapsed(now.Add(-2*time.Minute).Format(time.RFC3339), now, time.Minute) {
		t.Fatal("old timestamp should be ready")
	}
	if !cooldownElapsed("not-a-time", now, time.Minute) {
		t.Fatal("invalid timestamp should be ready")
	}
}
