package systemoneenable

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type memSettings struct {
	raw []byte
	ok  bool
}

func (m *memSettings) GetSetting(key string) ([]byte, bool, error) {
	if !m.ok {
		return nil, false, nil
	}
	return append([]byte(nil), m.raw...), true, nil
}

func (m *memSettings) UpsertSetting(key string, value []byte) error {
	m.raw = append([]byte(nil), value...)
	m.ok = true
	return nil
}

func TestEnableAPIAndRestore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer sk-cloud" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{"probe": map[string]any{"noul": 0.9}},
		})
	}))
	defer srv.Close()

	st := &memSettings{}
	var applied atomic.Int32
	var lastBase atomic.Value
	m := NewManager(st, func(cfg Config) {
		lastBase.Store(cfg.BaseURL)
		if cfg.BaseURL != "" {
			applied.Add(1)
		} else {
			applied.Store(0)
		}
	})

	if err := m.EnableAPI(context.Background(), EnableAPIRequest{
		BaseURL: srv.URL,
		Model:   "tev1",
		APIKey:  "sk-cloud",
	}); err != nil {
		t.Fatal(err)
	}
	snap := m.Snapshot(context.Background())
	if snap.Mode != "on" || snap.Provider != ProviderAPI || !snap.APIKeySet {
		t.Fatalf("snap=%+v", snap)
	}
	if applied.Load() == 0 {
		t.Fatal("config not applied")
	}
	if lastBase.Load().(string) != srv.URL {
		t.Fatalf("base=%v", lastBase.Load())
	}

	m2 := NewManager(st, func(cfg Config) {
		if cfg.BaseURL != "" {
			applied.Add(1)
		}
	})
	m2.Restore(context.Background())
	snap2 := m2.Snapshot(context.Background())
	if snap2.Mode != "on" || snap2.Provider != ProviderAPI {
		t.Fatalf("restore snap=%+v", snap2)
	}
}

func TestEnableAPIFailureDoesNotPersist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer srv.Close()

	st := &memSettings{}
	var applied atomic.Int32
	m := NewManager(st, func(cfg Config) {
		if cfg.BaseURL != "" {
			applied.Add(1)
		}
	})
	if err := m.EnableAPI(context.Background(), EnableAPIRequest{
		BaseURL: srv.URL,
		Model:   "tev1",
	}); err == nil {
		t.Fatal("expected probe error")
	}
	if applied.Load() != 0 {
		t.Fatal("must not apply on failure")
	}
	if st.ok {
		t.Fatal("must not persist on failure")
	}
	if snap := m.Snapshot(context.Background()); snap.Phase != PhaseFailed {
		t.Fatalf("phase=%s", snap.Phase)
	}
}

func TestEnableAPIKeepsPreviousKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-kept" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{"probe": map[string]any{"noul": 1}},
		})
	}))
	defer srv.Close()

	m := NewManager(&memSettings{}, nil)
	m.APIKey = "sk-kept"
	if err := m.EnableAPI(context.Background(), EnableAPIRequest{
		BaseURL: srv.URL,
		Model:   "tev1",
		APIKey:  "",
	}); err != nil {
		t.Fatal(err)
	}
	if m.APIKey != "sk-kept" {
		t.Fatalf("key=%q", m.APIKey)
	}
}

func TestLocalEnableFakeOllama(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"name": "tev1:latest"}},
			})
		case r.URL.Path == "/api/pull" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success"})
		case r.URL.Path == "/v1/systemone":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"answers": map[string]any{"probe": map[string]any{"noul": 0.8}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	st := &memSettings{}
	var gotBase atomic.Value
	m := NewManager(st, func(cfg Config) { gotBase.Store(cfg.BaseURL) })
	m.Base = srv.URL
	m.Model = "tev1"

	if err := m.StartEnable(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snap := m.Snapshot(context.Background())
		if snap.Phase == PhaseReady && snap.Mode == "on" {
			if gotBase.Load().(string) != srv.URL {
				t.Fatalf("applied base=%v", gotBase.Load())
			}
			return
		}
		if !snap.Busy && snap.Phase == PhaseFailed {
			t.Fatalf("failed: %s", snap.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout snap=%+v", m.Snapshot(context.Background()))
}
