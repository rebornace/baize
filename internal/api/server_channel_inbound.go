package api

import (
	"net/http"
	"time"

	"github.com/rebornace/baize/internal/store"
)

type channelInboundJobJSON struct {
	ID            string `json:"id"`
	Channel       string `json:"channel"`
	Status        string `json:"status"`
	Attempt       int    `json:"attempt"`
	MaxAttempts   int    `json:"max_attempts"`
	LastError     string `json:"last_error,omitempty"`
	NextAttemptAt string `json:"next_attempt_at,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

func channelInboundJobToJSON(job store.WebhookInboundJob) channelInboundJobJSON {
	out := channelInboundJobJSON{
		ID:          job.ID,
		Channel:     job.Channel,
		Status:      string(job.Status),
		Attempt:     job.Attempt,
		MaxAttempts: job.MaxAttempts,
		LastError:   job.LastError,
	}
	if !job.NextAttemptAt.IsZero() {
		out.NextAttemptAt = job.NextAttemptAt.UTC().Format(time.RFC3339)
	}
	if !job.CreatedAt.IsZero() {
		out.CreatedAt = job.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !job.UpdatedAt.IsZero() {
		out.UpdatedAt = job.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// handleListChannelInboundJobs lists dead webhook inbound jobs so operators can
// inspect and retry them. Bodies are deliberately omitted (may contain user
// content); only channel/attempt/error metadata is exposed.
func (s *Server) handleListChannelInboundJobs(w http.ResponseWriter, r *http.Request) {
	out := []channelInboundJobJSON{}
	if s.Store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"jobs": out})
		return
	}
	jobs, err := s.Store.ListWebhookInboundJobs("", []store.WebhookInboundStatus{store.WebhookInboundDead}, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	for _, job := range jobs {
		out = append(out, channelInboundJobToJSON(job))
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": out})
}

// handleRetryChannelInboundJob re-queues a dead job; the channel worker polls
// pending jobs and processes it within its normal retry loop.
func (s *Server) handleRetryChannelInboundJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "job id required")
		return
	}
	if s.Store == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "store unavailable")
		return
	}
	job, ok, err := s.Store.GetWebhookInboundJob(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "inbound job not found")
		return
	}
	if err := s.Store.ResetWebhookInboundJob(id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "pending",
		"id":      id,
		"channel": job.Channel,
	})
}
