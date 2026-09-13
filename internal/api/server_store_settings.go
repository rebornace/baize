package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/store"
)

type storeSettingsResponse struct {
	Driver      string   `json:"driver"`
	SQLitePath  string   `json:"sqlite_path,omitempty"`
	DSN         string   `json:"dsn,omitempty"`
	DSNRedacted string   `json:"dsn_redacted,omitempty"`
	Drivers     []string `json:"drivers"`
	ConfigPath  string   `json:"config_path,omitempty"`
	OverlayPath string   `json:"overlay_path,omitempty"`
}

type putStoreSettingsRequest struct {
	Driver               string `json:"driver"`
	SQLitePath           string `json:"sqlite_path"`
	DSN                  string `json:"dsn"`
	AcknowledgeNoMigrate bool   `json:"acknowledge_no_migrate"`
	Restart              bool   `json:"restart"`
}

func (s *Server) handleGetStoreSettings(w http.ResponseWriter, r *http.Request) {
	cfg := s.runtimeConfig()
	drivers := append([]string{"memory"}, store.ListDrivers()...)
	resp := storeSettingsResponse{
		Driver:      cfg.Store.Driver,
		SQLitePath:  cfg.Store.SQLitePath,
		DSNRedacted: config.RedactDSN(cfg.Store.DSN),
		Drivers:     drivers,
		ConfigPath:  s.ConfigPath,
	}
	if s.ConfigPath != "" {
		resp.OverlayPath = config.LocalOverlayPath(s.ConfigPath)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePutStoreSettings(w http.ResponseWriter, r *http.Request) {
	var body putStoreSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if !body.AcknowledgeNoMigrate {
		writeError(w, http.StatusBadRequest, "ack_required", "must acknowledge that data will not be migrated")
		return
	}
	driver := strings.ToLower(strings.TrimSpace(body.Driver))
	if driver == "" {
		writeError(w, http.StatusBadRequest, "invalid_driver", "driver is required")
		return
	}
	if driver != "memory" {
		found := false
		for _, d := range store.ListDrivers() {
			if d == driver {
				found = true
				break
			}
		}
		if !found {
			writeError(w, http.StatusBadRequest, "unknown_driver", "driver is not registered")
			return
		}
	}
	if driver == "postgres" && strings.TrimSpace(body.DSN) == "" {
		writeError(w, http.StatusBadRequest, "dsn_required", "dsn is required for postgres")
		return
	}
	if driver == "sqlite" && strings.TrimSpace(body.SQLitePath) == "" {
		body.SQLitePath = "./data/baize.db"
	}
	if strings.TrimSpace(s.ConfigPath) == "" {
		writeError(w, http.StatusBadRequest, "no_config_path", "server was started without a config path; edit YAML manually")
		return
	}
	overlay := config.StoreOverlay{
		Driver:     driver,
		SQLitePath: body.SQLitePath,
		DSN:        body.DSN,
	}
	if err := config.WriteStoreOverlay(s.ConfigPath, overlay); err != nil {
		writeError(w, http.StatusInternalServerError, "write_failed", err.Error())
		return
	}
	if body.Restart {
		go s.scheduleRestart()
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":  "restarting",
			"message": "store settings saved; process restart scheduled",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handlePostStoreRestart(w http.ResponseWriter, r *http.Request) {
	go s.scheduleRestart()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "restarting"})
}

func (s *Server) scheduleRestart() {
	time.Sleep(300 * time.Millisecond)
	if s.Shutdown != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	}
	if s.RestartProcess != nil {
		if err := s.RestartProcess(); err != nil {
			// Best effort: exit so process supervisor can restart.
			osExit(1)
		}
	}
}

// osExit is overridden in tests.
var osExit = func(code int) { os.Exit(code) }

func (s *Server) runtimeConfig() config.Config {
	if s.Config != nil {
		return *s.Config
	}
	return config.Config{}
}
