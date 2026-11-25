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

func TestRunRetentionSettingsAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	get := httptest.NewRequest(http.MethodGet, "/v0/settings/run-retention", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d", rr.Code)
	}
	var body struct {
		Days int `json:"days"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body.Days != 0 {
		t.Fatalf("default=%+v err=%v", body, err)
	}
	put := httptest.NewRequest(http.MethodPut, "/v0/settings/run-retention",
		bytes.NewBufferString(`{"days":30}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	if got := srv.runRetentionDays(); got != 30 {
		t.Fatalf("days=%d", got)
	}
	for _, payload := range []string{`{"days":-1}`, `{"days":3651}`} {
		bad := httptest.NewRequest(http.MethodPut, "/v0/settings/run-retention",
			bytes.NewBufferString(payload))
		bad.Header.Set("Authorization", "Bearer adm")
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, bad)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("bad %s=%d", payload, rr.Code)
		}
	}
}

func TestRunRetentionCleanup(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	if err := st.UpsertSetting(runRetentionSettingKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -40)
	oldRun, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "old", CreatedAt: old})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(oldRun.ID, store.StatusSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}
	recentRun, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "recent"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(recentRun.ID, store.StatusSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}
	waiting, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "waiting", CreatedAt: old})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(waiting.ID, store.StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}
	n, err := srv.cleanupExpiredRuns()
	if err != nil || n != 1 {
		t.Fatalf("cleanup n=%d err=%v", n, err)
	}
	if _, err := st.GetRun(oldRun.ID); err == nil {
		t.Fatal("old run still present")
	}
	if _, err := st.GetRun(recentRun.ID); err != nil {
		t.Fatalf("recent run removed: %v", err)
	}
	if got, err := st.GetRun(waiting.ID); err != nil || got.Status != store.StatusWaitingHuman {
		t.Fatalf("waiting run damaged: %+v err=%v", got, err)
	}
	if err := st.UpsertSetting(runRetentionSettingKey, []byte("0")); err != nil {
		t.Fatal(err)
	}
	if n, err := srv.cleanupExpiredRuns(); err != nil || n != 0 {
		t.Fatalf("off cleanup n=%d err=%v", n, err)
	}
}
