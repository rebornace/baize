package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

	channel, exists := s.Inbox.GetAny(channelID)
	if !exists {
		writeError(w, http.StatusNotFound, "channel_not_found", "unknown channel")
		return
	}
	if !channel.Enabled || strings.TrimSpace(channel.Secret) == "" {
		writeError(w, http.StatusNotFound, "channel_disabled", "channel disabled or secret missing")
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

	// Rate limit only after a valid signature so unsigned traffic cannot burn quota.
	allowed := false
	if s.InboxGate != nil {
		allowed = s.InboxGate(channelID)
	} else {
		allowed = s.inboxLimiter().Allow(channelID)
	}
	if !allowed {
		w.Header().Set("Retry-After", inboxRetryAfter)
		writeError(w, http.StatusTooManyRequests, "rate_limited", "channel rate limit exceeded")
		return
	}

	if proto := strings.TrimSpace(r.Header.Get("X-Baize-Protocol")); proto != "" && proto != "v0" {
		writeError(w, http.StatusBadRequest, "invalid_request", "unsupported protocol version")
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
	action := strings.TrimSpace(payload.Action)
	switch {
	case action == "" || action == inbox.ActionCreateRun:
		s.handleInboxCreateRun(w, r, channel, channelID, payload, bodyHash)
	case action == inbox.ActionResume:
		s.handleInboxResume(w, r, channel, channelID, payload, bodyHash)
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "unknown action")
	}
}

func (s *Server) handleInboxCreateRun(w http.ResponseWriter, r *http.Request, channel inbox.Channel, channelID string, payload inbox.Payload, bodyHash string) {
	idempotencyKey := strings.TrimSpace(payload.IdempotencyKey)
	deliveryID := "dlv_" + uuid.NewString()
	claimed := false

	if idempotencyKey != "" {
		if existing, found, err := s.Store.GetInboxDelivery(channelID, idempotencyKey); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		} else if found {
			if existing.BodyHash != bodyHash {
				writeError(w, http.StatusConflict, "idempotency_conflict", "idempotency key reused with different body")
				return
			}
			existing, err = waitInboxDeliveryRunID(s.Store, channelID, idempotencyKey, 2*time.Second)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
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

	if idempotencyKey != "" {
		// Claim the idempotency slot before startRun so concurrent POSTs cannot
		// create duplicate Runs. RunID is filled after startRun via UpdateInboxDelivery.
		if err := s.Store.PutInboxDelivery(store.InboxDelivery{
			ChannelID:      channelID,
			IdempotencyKey: idempotencyKey,
			DeliveryID:     deliveryID,
			RunID:          "",
			BodyHash:       bodyHash,
		}); err != nil {
			if errors.Is(err, store.ErrInboxDeliveryExists) {
				existing, found, getErr := s.Store.GetInboxDelivery(channelID, idempotencyKey)
				if getErr != nil {
					writeError(w, http.StatusInternalServerError, "internal_error", getErr.Error())
					return
				}
				if found {
					if existing.BodyHash != bodyHash {
						writeError(w, http.StatusConflict, "idempotency_conflict", "idempotency key reused with different body")
						return
					}
					existing, waitErr := waitInboxDeliveryRunID(s.Store, channelID, idempotencyKey, 2*time.Second)
					if waitErr != nil {
						writeError(w, http.StatusInternalServerError, "internal_error", waitErr.Error())
						return
					}
					convID := inboxReplayConversationID(s.Store, existing.RunID)
					writeInboxAccepted(w, http.StatusOK, existing.DeliveryID, existing.RunID, convID)
					return
				}
			}
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		claimed = true
	}

	convID := resolveConversation(s.Store, channel, payload)
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

	if claimed {
		if err := s.Store.UpdateInboxDelivery(channelID, idempotencyKey, runRec.ID); err != nil {
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

func (s *Server) handleInboxResume(w http.ResponseWriter, r *http.Request, channel inbox.Channel, channelID string, payload inbox.Payload, bodyHash string) {
	idempotencyKey := strings.TrimSpace(payload.IdempotencyKey)
	deliveryID := "dlv_" + uuid.NewString()

	if idempotencyKey != "" {
		if existing, found, err := s.Store.GetInboxDelivery(channelID, idempotencyKey); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		} else if found {
			if existing.BodyHash != bodyHash {
				writeError(w, http.StatusConflict, "idempotency_conflict", "idempotency key reused with different body")
				return
			}
			existing, err = waitInboxDeliveryRunID(s.Store, channelID, idempotencyKey, 2*time.Second)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
			status := ""
			if runRec, getErr := s.Store.GetRun(existing.RunID); getErr == nil && runRec != nil {
				status = string(runRec.Status)
			}
			writeInboxResumeOK(w, existing.DeliveryID, existing.RunID, status)
			return
		}
	}

	// Validate before claiming the idempotency slot so 404/403/409 cannot
	// leave an empty-RunID placeholder that poisons later replays.
	runID := strings.TrimSpace(payload.RunID)
	runRec, err := s.Store.GetRun(runID)
	if err != nil || runRec == nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if runRec.AgentID != channel.AgentID {
		writeError(w, http.StatusForbidden, "run_forbidden", "run does not belong to this channel agent")
		return
	}
	if runRec.Status != store.StatusWaitingHuman {
		writeError(w, http.StatusConflict, "not_waiting", "run is not waiting_human")
		return
	}

	if idempotencyKey != "" {
		// Claim after validation, with RunID filled, so concurrent same-key
		// POSTs lose the race and replay instead of double ContinueFromHITL.
		if err := s.Store.PutInboxDelivery(store.InboxDelivery{
			ChannelID:      channelID,
			IdempotencyKey: idempotencyKey,
			DeliveryID:     deliveryID,
			RunID:          runID,
			BodyHash:       bodyHash,
		}); err != nil {
			if errors.Is(err, store.ErrInboxDeliveryExists) {
				existing, found, getErr := s.Store.GetInboxDelivery(channelID, idempotencyKey)
				if getErr != nil {
					writeError(w, http.StatusInternalServerError, "internal_error", getErr.Error())
					return
				}
				if found {
					if existing.BodyHash != bodyHash {
						writeError(w, http.StatusConflict, "idempotency_conflict", "idempotency key reused with different body")
						return
					}
					existing, waitErr := waitInboxDeliveryRunID(s.Store, channelID, idempotencyKey, 2*time.Second)
					if waitErr != nil {
						writeError(w, http.StatusInternalServerError, "internal_error", waitErr.Error())
						return
					}
					status := ""
					if got, getErr := s.Store.GetRun(existing.RunID); getErr == nil && got != nil {
						status = string(got.Status)
					}
					writeInboxResumeOK(w, existing.DeliveryID, existing.RunID, status)
					return
				}
			}
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}

	decision := strings.TrimSpace(payload.Decision)
	approve := decision == "approve"
	_ = s.Runner.ContinueFromHITL(r.Context(), runID, run.Decision{
		Approve: approve,
		Comment: payload.Comment,
	})

	eventData := map[string]any{
		"channel_id":  channel.ID,
		"delivery_id": deliveryID,
		"decision":    decision,
	}
	if c := strings.TrimSpace(payload.Comment); c != "" {
		eventData["comment"] = c
	}
	if err := s.Store.AppendEvent(runID, store.Event{
		Type: run.EventInboxResumed,
		Data: eventData,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	updated, err := s.Store.GetRun(runID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "run missing after resume")
		return
	}
	writeInboxResumeOK(w, deliveryID, updated.ID, string(updated.Status))
}

func writeInboxResumeOK(w http.ResponseWriter, deliveryID, runID, status string) {
	writeJSON(w, http.StatusOK, map[string]any{
		"delivery_id": deliveryID,
		"run_id":      runID,
		"status":      status,
		"action":      "resume",
	})
}

// waitInboxDeliveryRunID polls until a claimed delivery has a non-empty RunID
// (winner of the Put claim finished UpdateInboxDelivery) or timeout.
func waitInboxDeliveryRunID(st store.Store, channelID, idempotencyKey string, timeout time.Duration) (store.InboxDelivery, error) {
	deadline := time.Now().Add(timeout)
	for {
		d, ok, err := st.GetInboxDelivery(channelID, idempotencyKey)
		if err != nil {
			return store.InboxDelivery{}, err
		}
		if ok && d.RunID != "" {
			return d, nil
		}
		if time.Now().After(deadline) {
			if ok {
				return d, errors.New("inbox delivery run_id not ready")
			}
			return store.InboxDelivery{}, errors.New("inbox delivery missing")
		}
		time.Sleep(5 * time.Millisecond)
	}
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

type inboxChannelView struct {
	ID             string            `json:"id"`
	AgentID        string            `json:"agent_id"`
	Enabled        bool              `json:"enabled"`
	Skills         []string          `json:"skills,omitempty"`
	Description    string            `json:"description,omitempty"`
	WebhookURL     string            `json:"webhook_url,omitempty"`
	WebhookHeaders map[string]string `json:"webhook_headers,omitempty"`
	SecretHint     string            `json:"secret_hint"`
}

type inboxChannelInput struct {
	ID             string            `json:"id"`
	AgentID        string            `json:"agent_id"`
	Enabled        *bool             `json:"enabled"`
	Skills         []string          `json:"skills,omitempty"`
	Description    string            `json:"description,omitempty"`
	WebhookURL     string            `json:"webhook_url,omitempty"`
	WebhookHeaders map[string]string `json:"webhook_headers,omitempty"`
	Secret         string            `json:"secret,omitempty"`
}

func (s *Server) loadInboxChannels() []inbox.Channel {
	raw, ok, err := s.Store.GetSetting(store.SettingKeyInboxChannels)
	if err != nil || !ok || len(raw) == 0 {
		return nil
	}
	var channels []inbox.Channel
	if err := json.Unmarshal(raw, &channels); err != nil {
		return nil
	}
	return channels
}

func (s *Server) persistInboxChannels(channels []inbox.Channel) error {
	raw, err := json.Marshal(channels)
	if err != nil {
		return err
	}
	if err := s.Store.UpsertSetting(store.SettingKeyInboxChannels, raw); err != nil {
		return err
	}
	if s.Inbox != nil {
		s.Inbox.Replace(channels)
	}
	return nil
}

func inboxChannelsToViews(channels []inbox.Channel) []inboxChannelView {
	if len(channels) == 0 {
		return []inboxChannelView{}
	}
	out := make([]inboxChannelView, 0, len(channels))
	for _, c := range channels {
		skills := c.Skills
		if skills == nil {
			skills = []string{}
		}
		headers := c.WebhookHeaders
		if headers == nil {
			headers = map[string]string{}
		}
		out = append(out, inboxChannelView{
			ID:             c.ID,
			AgentID:        c.AgentID,
			Enabled:        c.Enabled,
			Skills:         skills,
			Description:    c.Description,
			WebhookURL:     c.WebhookURL,
			WebhookHeaders: headers,
			SecretHint:     inbox.SecretHint(c.Secret),
		})
	}
	return out
}

func (s *Server) handleGetInboxChannels(w http.ResponseWriter, r *http.Request) {
	channels := s.loadInboxChannels()
	writeJSON(w, http.StatusOK, map[string]any{"channels": inboxChannelsToViews(channels)})
}

func (s *Server) handlePutInboxChannels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Channels []inboxChannelInput `json:"channels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}

	existingList := s.loadInboxChannels()
	existing := make(map[string]inbox.Channel, len(existingList))
	for _, c := range existingList {
		existing[c.ID] = c
	}

	seen := make(map[string]struct{}, len(body.Channels))
	merged := make([]inbox.Channel, 0, len(body.Channels))
	for _, in := range body.Channels {
		id := strings.TrimSpace(in.ID)
		if id == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "channel id is required")
			return
		}
		if _, dup := seen[id]; dup {
			writeError(w, http.StatusBadRequest, "invalid_request", "duplicate channel id: "+id)
			return
		}
		seen[id] = struct{}{}

		secret := strings.TrimSpace(in.Secret)
		if secret == "" {
			if prev, ok := existing[id]; ok {
				secret = prev.Secret
			} else {
				secret = inbox.GenerateSecret()
			}
		}

		enabled := true
		if in.Enabled != nil {
			enabled = *in.Enabled
		}

		c := inbox.Channel{
			ID:             id,
			AgentID:        strings.TrimSpace(in.AgentID),
			Enabled:        enabled,
			Skills:         in.Skills,
			Description:    in.Description,
			WebhookURL:     in.WebhookURL,
			WebhookHeaders: in.WebhookHeaders,
			Secret:         secret,
		}
		if err := c.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if _, err := s.Store.GetAgent(c.AgentID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "unknown agent: "+c.AgentID)
			return
		}
		merged = append(merged, c)
	}

	if err := s.persistInboxChannels(merged); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": inboxChannelsToViews(merged)})
}

func (s *Server) handlePostInboxRotateSecret(w http.ResponseWriter, r *http.Request) {
	channelID := strings.TrimSpace(r.PathValue("id"))
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing channel id")
		return
	}

	channels := s.loadInboxChannels()
	idx := -1
	for i, c := range channels {
		if c.ID == channelID {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeError(w, http.StatusNotFound, "channel_not_found", "unknown channel")
		return
	}

	newSecret := inbox.GenerateSecret()
	channels[idx].Secret = newSecret
	if err := s.persistInboxChannels(channels); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": newSecret})
}

func (s *Server) handlePostInboxTest(w http.ResponseWriter, r *http.Request) {
	if s.Inbox == nil {
		writeError(w, http.StatusNotFound, "channel_not_found", "inbox not configured")
		return
	}
	channelID := strings.TrimSpace(r.PathValue("id"))
	if channelID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing channel id")
		return
	}

	var channel inbox.Channel
	found := false
	for _, c := range s.loadInboxChannels() {
		if c.ID == channelID {
			channel = c
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "channel_not_found", "unknown channel")
		return
	}
	if !channel.Enabled || strings.TrimSpace(channel.Secret) == "" {
		writeError(w, http.StatusNotFound, "channel_disabled", "channel disabled or secret missing")
		return
	}

	body := []byte(`{"input":"inbox test"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := inbox.Sign(channel.Secret, ts, body)
	req := httptest.NewRequest(http.MethodPost, "/v0/inbox/"+channelID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Inbox-Timestamp", ts)
	req.Header.Set("X-Baize-Inbox-Signature", sig)
	req.SetPathValue("channel_id", channelID)

	rr := httptest.NewRecorder()
	s.handlePostInbox(rr, req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(rr.Code)
	_, _ = io.Copy(w, rr.Body)
}
