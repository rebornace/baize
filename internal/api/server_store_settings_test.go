package api_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/store"
)

func TestGetStoreSettings(t *testing.T) {
	cfg := config.Config{}
	cfg.Store.Driver = "sqlite"
	cfg.Store.SQLitePath = "./data/baize.db"
	srv := api.NewServer(store.NewMemory(), nil, nil)
	srv.AdminToken = "adm"
	srv.Config = &cfg
	srv.ConfigPath = "configs/minimal.yaml"

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/store", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Driver string `json:"driver"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Driver != "sqlite" {
		t.Fatalf("driver=%q", resp.Driver)
	}
}

func TestPutStoreSettingsRequiresAck(t *testing.T) {
	srv := api.NewServer(store.NewMemory(), nil, nil)
	srv.AdminToken = "adm"
	srv.ConfigPath = "configs/minimal.yaml"

	body := map[string]any{"driver": "sqlite", "sqlite_path": "./data/x.db"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/v0/settings/store", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestPutStoreSettingsHotSwap(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	if err := os.WriteFile(base, []byte("store:\n  driver: memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := api.NewServer(store.NewMemory(), nil, nil)
	srv.AdminToken = "adm"
	srv.ConfigPath = base
	var swapped config.StoreOverlay
	srv.HotSwapStore = func(o config.StoreOverlay) error {
		swapped = o
		return nil
	}

	body := map[string]any{
		"driver": "sqlite", "sqlite_path": "./data/y.db",
		"acknowledge_no_migrate": true,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/v0/settings/store", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "hot_swapped" {
		t.Fatalf("resp=%v", resp)
	}
	if swapped.Driver != "sqlite" {
		t.Fatalf("swapped=%+v", swapped)
	}
}

func TestPutStoreSettingsHotSwapFailureMarksOverlaySaved(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yaml")
	if err := os.WriteFile(base, []byte("store:\n  driver: memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := api.NewServer(store.NewMemory(), nil, nil)
	srv.AdminToken = "adm"
	srv.ConfigPath = base
	srv.HotSwapStore = func(config.StoreOverlay) error {
		return errors.New("boom")
	}

	body := map[string]any{
		"driver": "sqlite", "sqlite_path": "./data/z.db",
		"acknowledge_no_migrate": true,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/v0/settings/store", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rr.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["overlay_saved"] != true {
		t.Fatalf("resp=%v", resp)
	}
}

func TestPostSettingsReload(t *testing.T) {
	srv := api.NewServer(store.NewMemory(), nil, nil)
	srv.AdminToken = "adm"
	called := false
	srv.ReloadConfig = func() error {
		called = true
		return nil
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/settings/reload", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !called {
		t.Fatalf("status=%d called=%v body=%s", rr.Code, called, rr.Body.String())
	}
}
