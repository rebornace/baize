package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/channel/weixin"
)

type weixinLoginStartResponse struct {
	Ticket string `json:"ticket"`
	QRURL  string `json:"qr_url"`
}

type weixinLoginStatusResponse struct {
	Status string `json:"status"`
}

// weixinHandle looks up the registered weixin channel handle and asserts its
// concrete *weixin.Channel type. These handlers are weixin-specific, so the
// concrete type is asserted directly (future channels get their own handlers).
func (s *Server) weixinHandle() (*ChannelHandle, *weixin.Channel, bool) {
	h, ok := s.Channel(weixin.SourceName)
	if !ok {
		return nil, nil, false
	}
	ch, ok := h.Channel.(*weixin.Channel)
	if !ok {
		return nil, nil, false
	}
	return h, ch, true
}

// credsDir returns the on-disk creds/settings dir for the handle, falling back
// to the weixin package default when unset.
func (h *ChannelHandle) credsDir() string {
	if h == nil {
		return weixin.DefaultCredsDir
	}
	dir := strings.TrimSpace(h.CredsDir)
	if dir == "" {
		return weixin.DefaultCredsDir
	}
	return dir
}

// runCtx returns the long-lived channel Start context for the handle, falling
// back to context.Background when unset.
func (h *ChannelHandle) runCtx() context.Context {
	if h != nil && h.RunCtx != nil {
		return h.RunCtx
	}
	return context.Background()
}

func (s *Server) handleWeixinLoginStart(w http.ResponseWriter, r *http.Request) {
	_, ch, ok := s.weixinHandle()
	if !ok || ch.ILink() == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "weixin channel not configured")
		return
	}
	ticket, qrURL, err := ch.ILink().GetQR(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "ilink_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, weixinLoginStartResponse{Ticket: ticket, QRURL: qrURL})
}

func (s *Server) handleWeixinLoginStatus(w http.ResponseWriter, r *http.Request) {
	_, ch, ok := s.weixinHandle()
	if !ok || ch.ILink() == nil {
		writeError(w, http.StatusServiceUnavailable, "not_configured", "weixin channel not configured")
		return
	}
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" {
		writeError(w, http.StatusBadRequest, "missing_ticket", "ticket query parameter is required")
		return
	}
	status, accountID, token, err := ch.ILink().PollLogin(r.Context(), ticket)
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

	h, ch, ok := s.weixinHandle()
	if !ok {
		return nil
	}
	dir := h.credsDir()
	if err := weixin.SaveCreds(dir, accountID, token); err != nil {
		return err
	}
	ch.SetCredentials(accountID, token)
	if ch.IsStarted() {
		return nil
	}
	// Respect the persisted enabled state: an admin who disabled the channel
	// must not have the poll loop auto-started by a QR login. Credentials stay
	// saved so a later PUT enabled:true (via applyWeixinSettings) can Start.
	settings, err := weixin.LoadSettings(dir)
	if err != nil {
		// Don't regress the login experience on a settings read failure:
		// treat the channel as enabled and log a warning.
		log.Printf("weixin: cannot read persisted settings after login; starting channel as enabled: %v", err)
		return ch.Start(h.runCtx())
	}
	if !settings.Enabled {
		return nil
	}
	return ch.Start(h.runCtx())
}

func (s *Server) handleWeixinLogout(w http.ResponseWriter, r *http.Request) {
	s.weixinMu.Lock()
	defer s.weixinMu.Unlock()

	// Preserve legacy behavior: even when the channel is not wired we still
	// clear creds from the (default) dir and report logged_out.
	dir := weixin.DefaultCredsDir
	if h, ch, ok := s.weixinHandle(); ok {
		dir = h.credsDir()
		if ch.IsStarted() {
			if err := ch.Stop(r.Context()); err != nil {
				writeError(w, http.StatusInternalServerError, "stop_failed", err.Error())
				return
			}
		}
		ch.ClearCredentials()
	}
	if err := weixin.ClearCreds(dir); err != nil {
		writeError(w, http.StatusInternalServerError, "clear_creds_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (s *Server) handleGetWeixinSettings(w http.ResponseWriter, r *http.Request) {
	dir := weixin.DefaultCredsDir
	if h, _, ok := s.weixinHandle(); ok {
		dir = h.credsDir()
	}
	settings, err := weixin.LoadSettings(dir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read_failed", err.Error())
		return
	}
	if settings.Allowlist == nil {
		settings.Allowlist = []string{}
	}
	running, reason := s.weixinRuntimeState(settings)
	writeJSON(w, http.StatusOK, weixinSettingsResponse{
		Settings: settings,
		Running:  running,
		Reason:   reason,
	})
}

func (s *Server) handlePutWeixinSettings(w http.ResponseWriter, r *http.Request) {
	var body weixin.Settings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Allowlist == nil {
		body.Allowlist = []string{}
	}
	dir := weixin.DefaultCredsDir
	if h, _, ok := s.weixinHandle(); ok {
		dir = h.credsDir()
	}
	if err := weixin.SaveSettings(dir, body); err != nil {
		writeError(w, http.StatusInternalServerError, "write_failed", err.Error())
		return
	}
	running, reason := s.applyWeixinSettings(body)
	writeJSON(w, http.StatusOK, weixinSettingsResponse{
		Settings: body,
		Running:  running,
		Reason:   reason,
	})
}

// weixinSettingsResponse is the persisted settings plus the reconciled runtime
// state. Returned by both GET (current state) and PUT (state just applied).
type weixinSettingsResponse struct {
	weixin.Settings
	Running bool   `json:"running"`
	Reason  string `json:"reason,omitempty"` // "login_required" | "start_failed"
}

// weixinRuntimeState reports the channel's current running/reason without
// changing it, so GET can show status on first load (not only after a PUT).
func (s *Server) weixinRuntimeState(settings weixin.Settings) (running bool, reason string) {
	s.weixinMu.Lock()
	defer s.weixinMu.Unlock()
	_, ch, ok := s.weixinHandle()
	if !ok {
		return false, ""
	}
	if ch.IsStarted() {
		return true, ""
	}
	if !settings.Enabled {
		return false, "" // intentionally stopped
	}
	if !ch.HasCredentials() {
		return false, "login_required"
	}
	return false, "start_failed"
}

func (s *Server) applyWeixinSettings(settings weixin.Settings) (running bool, reason string) {
	s.weixinMu.Lock()
	defer s.weixinMu.Unlock()
	h, ch, ok := s.weixinHandle()
	if !ok {
		return false, ""
	}
	if h.Runtime != nil {
		if id := strings.TrimSpace(settings.Assignee); id != "" {
			h.Runtime.Assignee = id
		}
		if id := strings.TrimSpace(settings.AgentID); id != "" {
			h.Runtime.DefaultAgentID = id
		}
	}
	ch.SetAllowlist(settings.Allowlist)
	if settings.Enabled {
		if ch.IsStarted() {
			return true, ""
		}
		if !ch.HasCredentials() {
			return false, "login_required" // login success auto-starts when Enabled
		}
		if err := ch.Start(h.runCtx()); err != nil {
			return false, "start_failed"
		}
		return ch.IsStarted(), ""
	}
	// Disabled: stop the poll loop but KEEP credentials (unlike logout), so a
	// re-enable can Start directly without a new QR login.
	if ch.IsStarted() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ch.Stop(stopCtx)
	}
	return false, ""
}
