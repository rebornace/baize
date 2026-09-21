package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// TestPostRunEmitsModelRoutedEvent (DP-0): creating a run must persist a
// model.routed event recording the desired tier and the resolved profile. It
// is observability only — no routing outcome changes.
func TestPostRunEmitsModelRoutedEvent(t *testing.T) {
	setTestSettingsKey(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	light, err := st.UpsertModelProfile(store.ModelProfile{
		Name: "light-model", Provider: "openai_compatible",
		BaseURL: "https://x/v1", Model: "mini", AutoTier: store.AutoTierLight,
	})
	if err != nil {
		t.Fatal(err)
	}

	putAgent := httptest.NewRequest(http.MethodPut, "/v0/agents/test-agent",
		jsonBody(t, map[string]any{"system": "helper"}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, putAgent)
	if rr.Code != http.StatusOK {
		t.Fatalf("put agent status=%d body=%s", rr.Code, rr.Body.String())
	}

	postRun := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "test-agent", "input": "你好"}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, postRun)
	if rr.Code != http.StatusOK {
		t.Fatalf("post run status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)

	getEvs := httptest.NewRequest(http.MethodGet, "/v0/runs/"+runID+"/events", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, getEvs)
	if rr.Code != http.StatusOK {
		t.Fatalf("get events status=%d body=%s", rr.Code, rr.Body.String())
	}
	var evs []store.Event
	if err := json.Unmarshal(rr.Body.Bytes(), &evs); err != nil {
		t.Fatal(err)
	}
	var routed *store.Event
	for i := range evs {
		if evs[i].Type == "model.routed" {
			routed = &evs[i]
		}
	}
	if routed == nil {
		t.Fatalf("missing model.routed event; events=%+v", evs)
	}
	if got := routed.Data["desired_tier"]; got != store.AutoTierLight {
		t.Fatalf("desired_tier=%v want %s", got, store.AutoTierLight)
	}
	if got := routed.Data["resolved_profile_id"]; got != light.ID {
		t.Fatalf("resolved_profile_id=%v want %s", got, light.ID)
	}
	if got := routed.Data["auto"]; got != true {
		t.Fatalf("auto=%v want true", got)
	}
}
