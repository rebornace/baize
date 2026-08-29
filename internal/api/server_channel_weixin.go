package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rebornace/baize/internal/channel/weixin"
)

const (
	weixinSettingsFileName = "settings.json"
	// DefaultWeixinCredsDir is the default on-disk location for weixin creds + settings.
	DefaultWeixinCredsDir = "./data/channels/weixin"
)

// WeixinChannelSettings is persisted next to weixin creds (settings.json).
type WeixinChannelSettings struct {
	AgentID   string   `json:"agent_id"`
	Allowlist []string `json:"allowlist"`
	Assignee  string   `json:"assignee"`
	Enabled   bool     `json:"enabled"`
}

type weixinLoginStartResponse struct {
	Ticket string `json:"ticket"`
	QRURL  string `json:"qr_url"`
}

type weixinLoginStatusResponse struct {
	Status string `json:"status"`
}

func (s *Server) weixinCredsDir() string {
	dir := strings.TrimSpace(s.WeixinCredsDir)
	if dir == "" {
		return DefaultWeixinCredsDir
	}
	return dir
}

func (s *Server) handleWeixinLoginStart(w http.ResponseWriter, r *http.Request) {
	if s.WeixinILink == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "weixin channel not configured")
		return
	}
	ticket, qrURL, err := s.WeixinILink.GetQR(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "ilink_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, weixinLoginStartResponse{Ticket: ticket, QRURL: qrURL})
}

func (s *Server) handleWeixinLoginStatus(w http.ResponseWriter, r *http.Request) {
	if s.WeixinILink == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "weixin channel not configured")
		return
	}
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" {
		writeError(w, http.StatusBadRequest, "missing_ticket", "ticket query parameter is required")
		return
	}
	status, accountID, token, err := s.WeixinILink.PollLogin(r.Context(), ticket)
	if err != nil {
		writeError(w, http.StatusBadGateway, "ilink_error", err.Error())
		return
	}
	if status == weixin.LoginStatusSuccess {
		if err := s.applyWeixinLoginSuccess(accountID, token); err != nil {
			writeError(w, http.StatusInternalServerError, "login_apply_failed", err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, weixinLoginStatusResponse{Status: status})
}

func (s *Server) applyWeixinLoginSuccess(accountID, token string) error {
	s.weixinMu.Lock()
	defer s.weixinMu.Unlock()

	dir := s.weixinCredsDir()
	if err := weixin.SaveCreds(dir, accountID, token); err != nil {
		return err
	}
	if s.WeixinChannel == nil {
		return nil
	}
	s.WeixinChannel.SetCredentials(accountID, token)
	if s.WeixinChannel.IsStarted() {
		return nil
	}
	return s.WeixinChannel.Start(s.weixinRunCtx())
}

func (s *Server) handleWeixinLogout(w http.ResponseWriter, r *http.Request) {
	s.weixinMu.Lock()
	defer s.weixinMu.Unlock()

	if s.WeixinChannel != nil && s.WeixinChannel.IsStarted() {
		if err := s.WeixinChannel.Stop(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "stop_failed", err.Error())
			return
		}
	}
	if s.WeixinChannel != nil {
		s.WeixinChannel.ClearCredentials()
	}
	if err := weixin.ClearCreds(s.weixinCredsDir()); err != nil {
		writeError(w, http.StatusInternalServerError, "clear_creds_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (s *Server) handleGetWeixinSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := loadWeixinSettings(s.weixinCredsDir())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", err.Error())
		return
	}
	if settings.Allowlist == nil {
		settings.Allowlist = []string{}
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handlePutWeixinSettings(w http.ResponseWriter, r *http.Request) {
	var body WeixinChannelSettings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Allowlist == nil {
		body.Allowlist = []string{}
	}
	if err := saveWeixinSettings(s.weixinCredsDir(), body); err != nil {
		writeError(w, http.StatusInternalServerError, "write_failed", err.Error())
		return
	}
	s.applyWeixinSettings(body)
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) applyWeixinSettings(settings WeixinChannelSettings) {
	s.weixinMu.Lock()
	defer s.weixinMu.Unlock()
	if s.WeixinRuntime == nil {
		return
	}
	if id := strings.TrimSpace(settings.Assignee); id != "" {
		s.WeixinRuntime.Assignee = id
	}
	if id := strings.TrimSpace(settings.AgentID); id != "" {
		s.WeixinRuntime.DefaultAgentID = id
	}
}

func (s *Server) weixinRunCtx() context.Context {
	if s.WeixinRunCtx != nil {
		return s.WeixinRunCtx
	}
	return context.Background()
}

func loadWeixinSettings(dir string) (WeixinChannelSettings, error) {
	path := filepath.Join(dir, weixinSettingsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WeixinChannelSettings{Allowlist: []string{}, Enabled: true}, nil
		}
		return WeixinChannelSettings{}, err
	}
	var out WeixinChannelSettings
	if err := json.Unmarshal(data, &out); err != nil {
		return WeixinChannelSettings{}, err
	}
	if out.Allowlist == nil {
		out.Allowlist = []string{}
	}
	return out, nil
}

// LoadWeixinChannelSettings reads settings.json from dir (defaults when missing).
func LoadWeixinChannelSettings(dir string) (WeixinChannelSettings, error) {
	return loadWeixinSettings(dir)
}

func saveWeixinSettings(dir string, settings WeixinChannelSettings) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, weixinSettingsFileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
