package toolretrieval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rebornace/baize/internal/toolindex"
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
		if r.Header.Get("Authorization") != "Bearer sk-cloud" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float32{0.1, 0.2, 0.3}}},
		})
	}))
	defer srv.Close()

	st := &memSettings{}
	var applied atomic.Int32
	m := NewManager(st, func(emb toolindex.Embedder) {
		if emb != nil {
			applied.Add(1)
		} else {
			applied.Store(0)
		}
	})

	err := m.EnableAPI(context.Background(), EnableAPIRequest{
		BaseURL: srv.URL,
		Model:   "text-embedding-3-small",
		APIKey:  "sk-cloud",
	})
	if err != nil {
		t.Fatal(err)
	}
	snap := m.Snapshot(context.Background())
	if snap.Mode != "enhanced" || snap.Provider != ProviderAPI || !snap.APIKeySet {
		t.Fatalf("snap=%+v", snap)
	}
	if applied.Load() == 0 {
		t.Fatal("embedder not applied")
	}

	m2 := NewManager(st, func(emb toolindex.Embedder) {
		if emb != nil {
			applied.Add(1)
		}
	})
	m2.Restore(context.Background())
	snap2 := m2.Snapshot(context.Background())
	if snap2.Mode != "enhanced" || snap2.Provider != ProviderAPI {
		t.Fatalf("restore snap=%+v", snap2)
	}
}

func TestEnableAPIKeepsPreviousKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-kept" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float32{1}}},
		})
	}))
	defer srv.Close()

	m := NewManager(&memSettings{}, nil)
	m.APIKey = "sk-kept"
	if err := m.EnableAPI(context.Background(), EnableAPIRequest{
		BaseURL: srv.URL,
		Model:   "emb",
		APIKey:  "", // blank ⇒ keep previous
	}); err != nil {
		t.Fatal(err)
	}
	if m.APIKey != "sk-kept" {
		t.Fatalf("key=%q", m.APIKey)
	}
}
