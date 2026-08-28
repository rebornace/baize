package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

const (
	inboxMaxBodyBytes = 64 * 1024
	inboxMaxSkew      = 300 * time.Second
	inboxRetryAfter   = "60"
)

func (s *Server) handlePostInbox(w http.ResponseWriter, r *http.Request) {
	if s.Inbox == nil {
		writeError(w, http.StatusNotFound, "channel_not_found", "inbox not configured")
		return
	}

	channelID := strings.TrimSpace(r.PathValue("channel_id"))
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing channel id")
		return
	}

	channel, ok := s.Inbox.Get(channelID)
	if !ok {
		writeError(w, http.StatusNotFound, "channel_not_found", "unknown or disabled channel")
		return
	}

	rawBody, timestamp, sigHeader, err := readSignedInboxBody(r, inboxMaxBodyBytes)
	if err != nil {
		switch {
		case errors.Is(err, errInboxPayloadTooLarge):
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds 64 KiB")
		case errors.Is(err, errInboxMissingSig):
			writeError(w, http.StatusUnauthorized, "invalid_signature", "invalid inbox signature")
		default:
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}

	limiter := s.InboxLimiter
	if limiter == nil {
		limiter = inbox.NewRateLimiter(inbox.DefaultRateLimit, inbox.DefaultRateWindow)
	}
	if !limiter.Allow(channelID) {
		w.Header().Set("Retry-After", inboxRetryAfter)
		writeError(w, http.StatusTooManyRequests, "rate_limited", "channel rate limit exceeded")
		return
	}

	if proto := strings.TrimSpace(r.Header.Get("X-Baize-Protocol")); proto != "" && proto != "v0" {
		writeError(w, http.StatusBadRequest, "invalid_request", "unsupported protocol version")
		return
	}

	now := time.Now()
	if err := inbox.Verify(channel.Secret, timestamp, rawBody, sigHeader, now, inboxMaxSkew); err != nil {
		switch {
		case errors.Is(err, inbox.ErrTimestampSkew):
			writeError(w, http.StatusUnauthorized, "timestamp_skew", "request timestamp outside allowed window")
		default:
			writeError(w, http.StatusUnauthorized, "invalid_signature", "invalid inbox signature")
		}
		return
	}

	var payload inbox.Payload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if err := payload.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	bodyHash := sha256Hex(rawBody)
	idempotencyKey := strings.TrimSpace(payload.IdempotencyKey)
	if idempotencyKey != "" {
		if existing, found, err := s.Store.GetInboxDelivery(channelID, idempotencyKey); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		} else if found {
			if existing.BodyHash != bodyHash {
				writeError(w, http.StatusConflict, "idempotency_conflict", "idempotency key reused with different body")
				return
			}
			convID := inboxReplayConversationID(s.Store, existing.RunID)
			writeInboxAccepted(w, http.StatusOK, existing.DeliveryID, existing.RunID, convID)
			return
		}
	}

	if _, err := s.Store.GetAgent(channel.AgentID); err != nil {
		writeError(w, http.StatusNotFound, "agent_not_found", "unknown agent")
		return
	}

	convID := resolveConversation(s.Store, channel, payload)
	deliveryID := "dlv_" + uuid.NewString()
	inputText := strings.TrimSpace(payload.Input)

	var runSkills []string
	if len(channel.Skills) > 0 {
		runSkills = append([]string(nil), channel.Skills...)
	}

	var webhookCfg *store.WebhookConfig
	if u := strings.TrimSpace(channel.WebhookURL); u != "" || len(channel.WebhookHeaders) > 0 {
		webhookCfg = &store.WebhookConfig{
			URL:     u,
			Headers: channel.WebhookHeaders,
		}
	}

	preEventData := map[string]any{
		"channel_id":  channel.ID,
		"delivery_id": deliveryID,
	}
	if ext := strings.TrimSpace(payload.ExternalID); ext != "" {
		preEventData["external_id"] = ext
	}
	if idempotencyKey != "" {
		preEventData["idempotency_key"] = idempotencyKey
	}
	if len(payload.Metadata) > 0 {
		preEventData["metadata"] = payload.Metadata
	}

	runRec, err := s.startRun(r.Context(), startRunInput{
		AgentID:        channel.AgentID,
		Input:          inputText,
		ConversationID: convID,
		Skills:         runSkills,
		Webhook:        webhookCfg,
		PreEvents: []store.Event{{
			Type: run.EventInboxReceived,
			Data: preEventData,
		}},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if idempotencyKey != "" {
		d := store.InboxDelivery{
			ChannelID:      channelID,
			IdempotencyKey: idempotencyKey,
			DeliveryID:     deliveryID,
			RunID:          runRec.ID,
			BodyHash:       bodyHash,
		}
		if err := s.Store.PutInboxDelivery(d); err != nil {
			if existing, found, getErr := s.Store.GetInboxDelivery(channelID, idempotencyKey); getErr == nil && found {
				if existing.BodyHash == bodyHash {
					writeInboxAccepted(w, http.StatusOK, existing.DeliveryID, existing.RunID, convID)
					return
				}
				writeError(w, http.StatusConflict, "idempotency_conflict", "idempotency key reused with different body")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}

	if ext := strings.TrimSpace(payload.ExternalID); ext != "" && convID != "" {
		if err := s.Store.PutInboxThread(channel.ID, ext, convID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}

	writeInboxAccepted(w, http.StatusAccepted, deliveryID, runRec.ID, convID)
}

var (
	errInboxPayloadTooLarge = errors.New("inbox payload too large")
	errInboxMissingSig      = errors.New("missing inbox signature headers")
)

func readSignedInboxBody(r *http.Request, maxBytes int64) (body []byte, timestamp, sigHeader string, err error) {
	timestamp = strings.TrimSpace(r.Header.Get("X-Baize-Inbox-Timestamp"))
	sigHeader = strings.TrimSpace(r.Header.Get("X-Baize-Inbox-Signature"))
	if timestamp == "" || sigHeader == "" {
		return nil, "", "", errInboxMissingSig
	}

	limited := io.LimitReader(r.Body, maxBytes+1)
	body, err = io.ReadAll(limited)
	if err != nil {
		return nil, "", "", err
	}
	if int64(len(body)) > maxBytes {
		return nil, "", "", errInboxPayloadTooLarge
	}
	return body, timestamp, sigHeader, nil
}

func resolveConversation(st store.Store, channel inbox.Channel, payload inbox.Payload) string {
	if conv := strings.TrimSpace(payload.ConversationID); conv != "" {
		return conv
	}
	ext := strings.TrimSpace(payload.ExternalID)
	if ext == "" {
		return ""
	}
	if mapped, ok, err := st.GetInboxThread(channel.ID, ext); err == nil && ok {
		return mapped
	}
	return uuid.NewString()
}

func inboxReplayConversationID(st store.Store, runID string) string {
	runRec, err := st.GetRun(runID)
	if err != nil || runRec == nil {
		return ""
	}
	return runRec.ConversationID
}

func writeInboxAccepted(w http.ResponseWriter, status int, deliveryID, runID, conversationID string) {
	resp := map[string]any{
		"delivery_id": deliveryID,
		"run_id":      runID,
		"status":      "accepted",
	}
	if conversationID != "" {
		resp["conversation_id"] = conversationID
	}
	writeJSON(w, status, resp)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
