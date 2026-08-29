package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
