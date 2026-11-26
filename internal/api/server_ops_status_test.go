package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func TestOpsStatusAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	mk := func(status store.Status, conv string) {
		r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x", ConversationID: conv})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r.ID, status, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	mk(store.StatusRunning, "c1")
	mk(store.StatusWaitingHuman, "c2")
	mk(store.StatusSucceeded, "c3")
	mk(store.StatusFailed, "c4")
	_ = st.UpsertSetting(costAlertStateKey, []byte(`{"day_level":"warning","scopes":{"agent:a1":{"day_level":"exceeded"}}}`))
	_ = st.UpsertSetting(store.SettingKeyEventsWebhook, []byte(`{"url":"http://hook"}`))
	_ = st.UpsertSetting(attachmentRetentionKey, []byte("30"))
	_ = st.UpsertSetting(runRetentionSettingKey, []byte("60"))
	_ = st.UpsertSetting(messageRetentionSettingKey, []byte("90"))
	fresh, _ := json.Marshal(replicaHeartbeat{
		ID: "other-replica", At: time.Now().UTC(), Goroutines: 5,
	})
	_ = st.UpsertSetting(replicaStatusPrefix+"other-replica", fresh)
	stale, _ := json.Marshal(replicaHeartbeat{
		ID: "stale-replica", At: time.Now().UTC().Add(-2 * time.Minute), Goroutines: 9,
	})
	_ = st.UpsertSetting(replicaStatusPrefix+"stale-replica", stale)

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/ops-status", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Runs struct {
			Queued      int `json:"queued"`
			Running     int `json:"running"`
			Waiting     int `json:"waiting_human"`
			Active      int `json:"active"`
			Succeeded24 int `json:"succeeded_24h"`
			Failed24    int `json:"failed_24h"`
		} `json:"runs"`
		Webhook bool `json:"webhook_configured"`
		Budget  struct {
			Day    string `json:"day_alert"`
			Scoped int    `json:"scoped_alerts"`
		} `json:"budget"`
		Retention struct {
			Attachment int `json:"attachment_days"`
			Run        int `json:"run_days"`
			Message    int `json:"message_days"`
		} `json:"retention"`
		Process struct {
			Goroutines int `json:"goroutines"`
		} `json:"process"`
		DB struct {
			MaxOpen int `json:"max_open"`
			Open    int `json:"open"`
			InUse   int `json:"in_use"`
			Idle    int `json:"idle"`
		} `json:"db"`
		Streams struct {
			Live    int64 `json:"live"`
			Dropped int64 `json:"dropped"`
			Slow    int64 `json:"slow"`
		} `json:"streams"`
		Replicas []struct {
			ID         string `json:"id"`
			Goroutines int    `json:"goroutines"`
		} `json:"replicas"`
		ReplicaCount int `json:"replica_count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Runs.Running != 1 || body.Runs.Waiting != 1 || body.Runs.Active != 2 ||
		body.Runs.Succeeded24 != 1 || body.Runs.Failed24 != 1 {
		t.Fatalf("runs=%+v", body.Runs)
	}
	if !body.Webhook || body.Budget.Day != "warning" || body.Budget.Scoped != 1 {
		t.Fatalf("webhook/budget=%+v", body)
	}
	if body.Retention.Attachment != 30 || body.Retention.Run != 60 || body.Retention.Message != 90 {
		t.Fatalf("retention=%+v", body.Retention)
	}
	if body.Process.Goroutines <= 0 {
		t.Fatalf("process=%+v", body.Process)
	}
	if body.DB.Open < 0 || body.DB.InUse < 0 || body.DB.Idle < 0 {
		t.Fatalf("db=%+v", body.DB)
	}
	if body.Streams.Live != 0 || body.Streams.Dropped != 0 || body.Streams.Slow != 0 {
		t.Fatalf("streams=%+v", body.Streams)
	}
	if body.ReplicaCount < 2 {
		t.Fatalf("replica_count=%d replicas=%+v", body.ReplicaCount, body.Replicas)
	}
	foundOther, foundStale := false, false
	for _, rep := range body.Replicas {
		switch rep.ID {
		case "other-replica":
			foundOther = true
		case "stale-replica":
			foundStale = true
		}
	}
	if !foundOther || foundStale {
		t.Fatalf("replicas=%+v", body.Replicas)
	}
}
