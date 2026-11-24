package api

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNightlyEvalGateRunsOncePerDay(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	now := time.Now().UTC()
	hourRaw, _ := json.Marshal(now.Hour())
	if err := srv.Store.UpsertSetting(evalGateAutoHourKey, hourRaw); err != nil {
		t.Fatal(err)
	}
	ran, err := srv.runNightlyGateIfDue(now)
	if err != nil || !ran {
		t.Fatalf("first run=%v err=%v", ran, err)
	}
	if srv.evalGateLastResult() == nil {
		t.Fatal("nightly gate did not persist a result")
	}
	ran, err = srv.runNightlyGateIfDue(now.Add(5 * time.Minute))
	if err != nil || ran {
		t.Fatalf("second run=%v err=%v", ran, err)
	}
	// A different hour with auto still set does not run.
	ran, err = srv.runNightlyGateIfDue(now.Add(time.Hour))
	if err != nil || ran {
		t.Fatalf("wrong hour run=%v err=%v", ran, err)
	}
}

func TestEvalGateAutoHourOffByDefault(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	if got := srv.evalGateAutoHour(); got != -1 {
		t.Fatalf("auto hour=%d, want -1", got)
	}
	if ran, err := srv.runNightlyGateIfDue(time.Now().UTC()); err != nil || ran {
		t.Fatalf("disabled ran=%v err=%v", ran, err)
	}
}
