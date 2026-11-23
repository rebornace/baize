package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/channel"
)

// managedChannel resolves a registered channel that implements the generic
// management plane. It writes 404 and returns false when the name is unknown
// or the channel does not expose management (third-party / non-managed
// channels).
func (s *Server) managedChannel(w http.ResponseWriter, name string) (channel.ManagedChannel, bool) {
	h, ok := s.Channel(name)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "channel not configured")
		return nil, false
	}
	mc, ok := h.Channel.(channel.ManagedChannel)
	if !ok {
		writeError(w, http.StatusNotFound, "not_managed", "channel has no management plane")
		return nil, false
	}
	return mc, true
}

// settingsResponse is settings plus reconciled status, matching the historical
// channel settings JSON contract (agent_id/allowlist/assignee/enabled plus
// running/reason).
type settingsResponse struct {
	channel.ChannelSettings
	Running bool   `json:"running"`
	Reason  string `json:"reason,omitempty"`
}

func (s *Server) handleGetChannelSettings(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.managedChannel(w, r.PathValue("name"))
	if !ok {
		return
	}
	st := mc.GetSettings()
	status := mc.Status()
	writeJSON(w, http.StatusOK, settingsResponse{ChannelSettings: st, Running: status.Running, Reason: status.Reason})
}

func (s *Server) handlePutChannelSettings(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.managedChannel(w, r.PathValue("name"))
	if !ok {
		return
	}
	var body channel.ChannelSettings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Allowlist == nil {
		body.Allowlist = []string{}
	}
	status := mc.UpdateSettings(body)
	writeJSON(w, http.StatusOK, settingsResponse{ChannelSettings: body, Running: status.Running, Reason: status.Reason})
}

func (s *Server) handleChannelLoginStart(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.managedChannel(w, r.PathValue("name"))
	if !ok {
		return
	}
	ticket, err := mc.LoginStart(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "adapter_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ticket": ticket.Ticket, "qr_url": ticket.QRURL})
}

func (s *Server) handleChannelLoginStatus(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.managedChannel(w, r.PathValue("name"))
	if !ok {
		return
	}
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if ticket == "" {
		writeError(w, http.StatusBadRequest, "missing_ticket", "ticket query parameter is required")
		return
	}
	status, err := mc.LoginPoll(r.Context(), ticket)
	if err != nil {
		writeError(w, http.StatusBadGateway, "adapter_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (s *Server) handleChannelLogout(w http.ResponseWriter, r *http.Request) {
	mc, ok := s.managedChannel(w, r.PathValue("name"))
	if !ok {
		return
	}
	if err := mc.Logout(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, "adapter_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// processController resolves a managed channel that also exposes the process
// control plane. Channels with an independently deployed adapter do not.
func (s *Server) processController(w http.ResponseWriter, name string) (channel.ManagedChannel, channel.ProcessController, bool) {
	mc, ok := s.managedChannel(w, name)
	if !ok {
		return nil, nil, false
	}
	pc, ok := mc.(channel.ProcessController)
	if !ok {
		writeError(w, http.StatusNotImplemented, "not_controllable", "channel adapter process is not managed by baize")
		return nil, nil, false
	}
	return mc, pc, true
}

// handleChannelProcessStart launches (or adopts) the adapter process.
func (s *Server) handleChannelProcessStart(w http.ResponseWriter, r *http.Request) {
	mc, pc, ok := s.processController(w, r.PathValue("name"))
	if !ok {
		return
	}
	if err := pc.StartProcess(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, "adapter_error", err.Error())
		return
	}
	st := mc.GetSettings()
	status := mc.Status()
	writeJSON(w, http.StatusOK, settingsResponse{ChannelSettings: st, Running: status.Running, Reason: status.Reason})
}

// handleChannelProcessStop terminates the adapter process.
func (s *Server) handleChannelProcessStop(w http.ResponseWriter, r *http.Request) {
	mc, pc, ok := s.processController(w, r.PathValue("name"))
	if !ok {
		return
	}
	if err := pc.StopProcess(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, "adapter_error", err.Error())
		return
	}
	st := mc.GetSettings()
	writeJSON(w, http.StatusOK, settingsResponse{ChannelSettings: st, Running: false, Reason: "stopped"})
}

// handleChannelProcessRestart kills and relaunches the adapter process.
func (s *Server) handleChannelProcessRestart(w http.ResponseWriter, r *http.Request) {
	mc, pc, ok := s.processController(w, r.PathValue("name"))
	if !ok {
		return
	}
	if err := pc.RestartProcess(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, "adapter_error", err.Error())
		return
	}
	st := mc.GetSettings()
	status := mc.Status()
	writeJSON(w, http.StatusOK, settingsResponse{ChannelSettings: st, Running: status.Running, Reason: status.Reason})
}
