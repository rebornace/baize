package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/webhook"
)

func (s *Server) loadEventsWebhook() webhook.Config {
	raw, ok, err := s.Store.GetSetting(store.SettingKeyEventsWebhook)
	if err != nil || !ok || len(raw) == 0 {
		return webhook.Config{}
	}
	var cfg webhook.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return webhook.Config{}
	}
	return cfg
}

func (s *Server) handleGetEventsWebhook(w http.ResponseWriter, r *http.Request) {
	cfg := s.loadEventsWebhook()
	if cfg.Headers == nil {
		cfg.Headers = map[string]string{}
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handlePutEventsWebhook(w http.ResponseWriter, r *http.Request) {
	var body webhook.Config
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if body.Headers == nil {
		body.Headers = map[string]string{}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := s.Store.UpsertSetting(store.SettingKeyEventsWebhook, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if s.Webhook != nil {
		s.Webhook.UpdateConfig(body)
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) handlePostEventsWebhookTest(w http.ResponseWriter, r *http.Request) {
	if s.Webhook == nil {
		writeError(w, http.StatusServiceUnavailable, "webhook_unavailable", "webhook dispatcher is not configured")
		return
	}
	if err := s.Webhook.SendTest(r.Context()); err != nil {
		switch {
		case errors.Is(err, webhook.ErrNoURL):
			writeError(w, http.StatusBadRequest, "webhook_not_configured", "webhook url is not configured")
		case errors.Is(err, authcred.ErrInvalidAuth):
			writeError(w, http.StatusBadRequest, "invalid_auth", err.Error())
		default:
			writeError(w, http.StatusBadGateway, "webhook_delivery_failed", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetEventsWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	statusParam := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	var statuses []store.WebhookOutboxStatus
	if statusParam == "" {
		statuses = []store.WebhookOutboxStatus{store.WebhookOutboxDead, store.WebhookOutboxPending}
	} else {
		for _, part := range strings.Split(statusParam, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			statuses = append(statuses, store.WebhookOutboxStatus(part))
		}
	}
	entries, err := s.Store.ListWebhookOutbox(statuses, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	type row struct {
		ID          string `json:"id"`
		RunID       string `json:"run_id"`
		Kind        string `json:"kind"`
		EventIndex  int    `json:"event_index"`
		Status      string `json:"status"`
		Attempt     int    `json:"attempt"`
		MaxAttempts int    `json:"max_attempts"`
		LastError   string `json:"last_error,omitempty"`
		NextRetryAt string `json:"next_retry_at"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
	}
	out := make([]row, 0, len(entries))
	for _, e := range entries {
		out = append(out, row{
			ID:          e.ID,
			RunID:       e.RunID,
			Kind:        string(e.Kind),
			EventIndex:  e.EventIndex,
			Status:      string(e.Status),
			Attempt:     e.Attempt,
			MaxAttempts: e.MaxAttempts,
			LastError:   e.LastError,
			NextRetryAt: e.NextRetryAt.UTC().Format(time.RFC3339Nano),
			CreatedAt:   e.CreatedAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:   e.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": out})
}

func (s *Server) handlePostEventsWebhookDeliveryRetry(w http.ResponseWriter, r *http.Request) {
	if s.Webhook == nil {
		writeError(w, http.StatusServiceUnavailable, "webhook_unavailable", "webhook dispatcher is not configured")
		return
	}
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing delivery id")
		return
	}
	if err := s.Webhook.RetryDelivery(id); err != nil {
		if errors.Is(err, store.ErrWebhookOutboxNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "delivery not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
}

// channelOutboundRetrier is implemented by webhook.Channel for manual outbox retry.
type channelOutboundRetrier interface {
	RetryOutbound(id string) error
}

func (s *Server) handleGetChannelOutboundDeliveries(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing channel name")
		return
	}
	if _, ok := s.Channel(name); !ok {
		writeError(w, http.StatusNotFound, "not_found", "channel not configured")
		return
	}
	statusParam := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	var statuses []store.ChannelOutboxStatus
	if statusParam == "" {
		statuses = []store.ChannelOutboxStatus{store.ChannelOutboxDead, store.ChannelOutboxPending}
	} else {
		for _, part := range strings.Split(statusParam, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			statuses = append(statuses, store.ChannelOutboxStatus(part))
		}
	}
	entries, err := s.Store.ListChannelOutbox(name, statuses, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	type row struct {
		ID             string `json:"id"`
		Status         string `json:"status"`
		Kind           string `json:"kind"`
		PeerID         string `json:"peer_id,omitempty"`
		ConversationID string `json:"conversation_id,omitempty"`
		RunID          string `json:"run_id,omitempty"`
		Attempt        int    `json:"attempt"`
		MaxAttempts    int    `json:"max_attempts"`
		LastError      string `json:"last_error,omitempty"`
		UpdatedAt      string `json:"updated_at"`
	}
	out := make([]row, 0, len(entries))
	for _, e := range entries {
		out = append(out, row{
			ID:             e.ID,
			Status:         string(e.Status),
			Kind:           string(e.Kind),
			PeerID:         e.PeerID,
			ConversationID: e.ConversationID,
			RunID:          e.RunID,
			Attempt:        e.Attempt,
			MaxAttempts:    e.MaxAttempts,
			LastError:      e.LastError,
			UpdatedAt:      e.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": out})
}

func (s *Server) handlePostChannelOutboundDeliveryRetry(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing channel name")
		return
	}
	h, ok := s.Channel(name)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "channel not configured")
		return
	}
	retrier, ok := h.Channel.(channelOutboundRetrier)
	if !ok {
		writeError(w, http.StatusNotImplemented, "not_supported", "channel does not support outbound retry")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing delivery id")
		return
	}
	if err := retrier.RetryOutbound(id); err != nil {
		if errors.Is(err, store.ErrChannelOutboxNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "delivery not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
}
