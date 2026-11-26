package api

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestAgentRunRetentionSettingsAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	srv.Store.UpsertAgent(store.Agent{ID: "a1", System: "s"})
	srv.Store.UpsertAgent(store.Agent{ID: "a2", System: "s"})
	h := srv.Handler()

	get := httptest.NewRequest(http.MethodGet, "/v0/settings/agent-run-retention", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		GlobalDays int `json:"global_days"`
		Items      []struct {
			AgentID string `json:"agent_id"`
			Days    int    `json:"days"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 || body.Items[0].Days != -1 {
		t.Fatalf("items=%+v", body.Items)
	}

	put := httptest.NewRequest(http.MethodPut, "/v0/settings/agent-run-retention",
		bytes.NewBufferString(`{"agent_id":"a1","days":7}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	if d, ok := srv.runRetentionDaysForAgent("a1"); !ok || d != 7 {
		t.Fatalf("agent days=%d ok=%v", d, ok)
	}

	bad := httptest.NewRequest(http.MethodPut, "/v0/settings/agent-run-retention",
		bytes.NewBufferString(`{"agent_id":"a1","days":-2}`))
	bad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad days=%d", rr.Code)
	}

	missing := httptest.NewRequest(http.MethodPut, "/v0/settings/agent-run-retention",
		bytes.NewBufferString(`{"agent_id":"nope","days":7}`))
	missing.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, missing)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing agent=%d", rr.Code)
	}
}

func TestAgentRunRetentionCleanup(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	st.UpsertAgent(store.Agent{ID: "a1", System: "s"})
	st.UpsertAgent(store.Agent{ID: "a2", System: "s"})
	old := time.Now().UTC().AddDate(0, 0, -40)
	mk := func(agentID, conv string) *store.Run {
		t.Helper()
		r, err := st.CreateRun(store.CreateRunInput{
			AgentID: agentID, Input: "x", ConversationID: conv, CreatedAt: old,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r.ID, store.StatusSucceeded, "ok", ""); err != nil {
			t.Fatal(err)
		}
		return r
	}
	a1Old := mk("a1", "c-a1")
	a2Old := mk("a2", "c-a2")
	a1Forever := mk("a1", "c-a1-forever")
	a2Short := mk("a2", "c-a2-short")
	if err := st.UpsertSetting(runRetentionSettingKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(runRetentionAgentPrefix+"a1", []byte("7")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(runRetentionAgentPrefix+"a2", []byte("0")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(runRetentionConvPrefix+"c-a1-forever", []byte("0")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(runRetentionConvPrefix+"c-a2-short", []byte("3")); err != nil {
		t.Fatal(err)
	}

	if _, err := srv.cleanupExpiredRuns(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetRun(a1Old.ID); err == nil {
		t.Fatal("a1 run should use the agent 7-day window")
	}
	if _, err := st.GetRun(a2Old.ID); err != nil {
		t.Fatalf("a2 agent 0 should keep the run: %v", err)
	}
	if _, err := st.GetRun(a1Forever.ID); err != nil {
		t.Fatalf("conversation 0 should win over agent 7: %v", err)
	}
	if _, err := st.GetRun(a2Short.ID); err == nil {
		t.Fatal("conversation 3 should win over agent 0")
	}
}

func TestConversationRunRetentionAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	if err := srv.Store.UpsertSetting(runRetentionSettingKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/run-retention", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Days      int `json:"days"`
		Effective int `json:"effective_days"`
		Global    int `json:"global_days"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Days != -1 || body.Effective != 30 || body.Global != 30 {
		t.Fatalf("default=%+v", body)
	}
	put := httptest.NewRequest(http.MethodPut, "/v0/conversations/c1/run-retention",
		bytes.NewBufferString(`{"days":90}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Days != 90 || body.Effective != 90 || body.Global != 30 {
		t.Fatalf("override=%+v", body)
	}
	bad := httptest.NewRequest(http.MethodPut, "/v0/conversations/c1/run-retention",
		bytes.NewBufferString(`{"days":3651}`))
	bad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad=%d", rr.Code)
	}
}

func TestRunRetentionConversationOverride(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	if err := st.UpsertSetting(runRetentionSettingKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(runRetentionConvPrefix+"c-long", []byte("90")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(runRetentionConvPrefix+"c-forever", []byte("0")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(runRetentionConvPrefix+"c-short", []byte("10")); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -40)
	mk := func(conv string) *store.Run {
		r, err := st.CreateRun(store.CreateRunInput{
			AgentID: "a", Input: conv, ConversationID: conv, CreatedAt: old,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r.ID, store.StatusSucceeded, "ok", ""); err != nil {
			t.Fatal(err)
		}
		return r
	}
	plain := mk("c-plain")
	long := mk("c-long")
	forever := mk("c-forever")
	short := mk("c-short")

	n, err := srv.cleanupExpiredRuns()
	if err != nil || n != 2 {
		t.Fatalf("cleanup n=%d err=%v", n, err)
	}
	if _, err := st.GetRun(plain.ID); err == nil {
		t.Fatal("global conversation not cleaned")
	}
	if _, err := st.GetRun(short.ID); err == nil {
		t.Fatal("short override not cleaned")
	}
	if _, err := st.GetRun(long.ID); err != nil {
		t.Fatalf("long override removed: %v", err)
	}
	if _, err := st.GetRun(forever.ID); err != nil {
		t.Fatalf("forever override removed: %v", err)
	}

	// Turning the global policy off must not stop scoped overrides.
	if err := st.UpsertSetting(runRetentionSettingKey, []byte("0")); err != nil {
		t.Fatal(err)
	}
	if n, err := srv.cleanupExpiredRuns(); err != nil || n != 0 {
		t.Fatalf("global-off cleanup n=%d err=%v", n, err)
	}
	if _, err := st.GetRun(long.ID); err != nil {
		t.Fatalf("long override damaged: %v", err)
	}
	if _, err := st.GetRun(forever.ID); err != nil {
		t.Fatalf("forever override damaged: %v", err)
	}
}

func TestRunRetentionManyOverridesFallback(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	// Force the per-conversation fallback with a large override set.
	for i := 0; i <= runRetentionMaxOverrides; i++ {
		key := fmt.Sprintf("%sbulk-%d", runRetentionConvPrefix, i)
		if err := st.UpsertSetting(key, []byte("0")); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.UpsertSetting(runRetentionConvPrefix+"c-old", []byte("1")); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -40)
	mk := func(conv string) {
		r, err := st.CreateRun(store.CreateRunInput{
			AgentID: "a", Input: conv, ConversationID: conv, CreatedAt: old,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r.ID, store.StatusSucceeded, "ok", ""); err != nil {
			t.Fatal(err)
		}
	}
	mk("c-old")
	mk("c-plain")
	n, err := srv.cleanupExpiredRuns()
	if err != nil || n != 1 {
		t.Fatalf("fallback n=%d err=%v", n, err)
	}
	convs, err := st.ListRunConversationIDs()
	if err != nil {
		t.Fatal(err)
	}
	for _, conv := range convs {
		switch conv {
		case "c-old":
			t.Fatal("c-old should be cleaned")
		case "c-plain":
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
