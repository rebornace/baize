package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

// newUpstreamModelsServer mocks an OpenAI-compatible provider and serves a
// fixed model catalog.
func newUpstreamModelsServer(t *testing.T, models []llm.UpstreamModel) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": models})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDiscoverModelsSuccess(t *testing.T) {
	srv := modelProfilesServer(t)
	h := srv.Handler()
	upstream := newUpstreamModelsServer(t, []llm.UpstreamModel{
		{ID: "model-a"}, {ID: "model-b"}, {ID: "" /* filtered */},
	})

	body := `{"base_url":"` + upstream.URL + `/v1","api_key":"sk-upstream-1234"}`
	req := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models/discover", strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("discover: code=%d body=%s", rr.Code, rr.Body)
	}
	var out struct {
		Models []llm.UpstreamModel `json:"models"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Models) != 2 {
		t.Fatalf("want 2 models (empty id filtered), got %d: %+v", len(out.Models), out.Models)
	}
	if out.Models[0].ID != "model-a" || out.Models[1].ID != "model-b" {
		t.Fatalf("unexpected models: %+v", out.Models)
	}
}

func TestDiscoverModelsRequiresBaseURL(t *testing.T) {
	srv := modelProfilesServer(t)
	h := srv.Handler()

	req := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models/discover",
		strings.NewReader(`{"api_key":"k"}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body=%s", rr.Code, rr.Body)
	}
}

func TestDiscoverModelsFromExistingProfileReusesStoredKey(t *testing.T) {
	srv := modelProfilesServer(t)
	h := srv.Handler()

	// Seed a profile with a real key.
	seed := store.ModelProfile{
		Name: "seeded", Provider: supportedModelProvider,
		BaseURL: "http://example.invalid/v1", Model: "m0", APIKey: "sk-stored-1234",
		AutoTier: store.AutoTierStandard,
	}
	saved, err := srv.Store.UpsertModelProfile(seed)
	if err != nil {
		t.Fatal(err)
	}

	var captured string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "x1"}}})
	}))
	defer upstream.Close()

	// Update the seeded base URL via the discover call but omit api_key.
	body := `{"base_url":"` + upstream.URL + `","profile_id":"` + saved.ID + `"}`
	req := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models/discover", strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body)
	}
	if captured != "Bearer sk-stored-1234" {
		t.Fatalf("stored key not reused, Authorization=%q", captured)
	}
}

func TestBatchImportCreatesProfiles(t *testing.T) {
	srv := modelProfilesServer(t)
	h := srv.Handler()

	body := `{
	  "base_url":"https://up.example/v1",
	  "api_key":"sk-batch-1234",
	  "models":[{"id":"light-model"},{"id":"deep-reasoner"},{"id":"dup-name"}]
	}`
	// Pre-create a profile named "dup-name" to force one skip.
	if _, err := srv.Store.UpsertModelProfile(store.ModelProfile{
		Name: "dup-name", Provider: supportedModelProvider, BaseURL: "https://up.example/v1",
		Model: "other", APIKey: "sk-batch-1234", AutoTier: store.AutoTierStandard,
	}); err != nil {
		t.Fatal(err)
	}

	req := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models/batch", strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("batch: code=%d body=%s", rr.Code, rr.Body)
	}

	var out struct {
		Result batchImportResult `json:"result"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Result.Created) != 2 {
		t.Fatalf("want 2 created, got %d", len(out.Result.Created))
	}
	if len(out.Result.Skipped) != 1 || out.Result.Skipped[0].ID != "dup-name" {
		t.Fatalf("want 1 skip for dup-name, got %+v", out.Result.Skipped)
	}

	// Keys must be encrypted at rest; listing returns them masked.
	list, err := srv.Store.ListModelProfiles()
	if err != nil {
		t.Fatal(err)
	}
	// seeded dup-name + 2 new = 3
	if len(list) != 3 {
		t.Fatalf("want 3 stored profiles, got %d", len(list))
	}

	// The created profiles should carry an inferred tier.
	tierByName := map[string]string{}
	for _, p := range out.Result.Created {
		tierByName[p.Model] = p.AutoTier
	}
	if tierByName["light-model"] == "" || tierByName["deep-reasoner"] == "" {
		t.Fatalf("tiers should be inferred: %+v", tierByName)
	}
}

func TestBatchImportRequiresSelection(t *testing.T) {
	srv := modelProfilesServer(t)
	h := srv.Handler()

	body := `{"base_url":"https://up.example/v1","api_key":"k","models":[]}`
	req := withAdmin(httptest.NewRequest(http.MethodPost, "/v0/settings/models/batch", strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body=%s", rr.Code, rr.Body)
	}
}
