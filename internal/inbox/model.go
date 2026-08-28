package inbox

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var channelIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

const (
	ActionCreateRun = "create_run"
	ActionResume    = "resume"

	maxPayloadInputLen   = 8192
	maxIdempotencyKeyLen = 128
	maxExternalIDLen     = 256
)

// Channel is an inbound webhook entry binding an agent and signing secret.
type Channel struct {
	ID             string            `json:"id"`
	AgentID        string            `json:"agent_id"`
	Enabled        bool              `json:"enabled"`
	Skills         []string          `json:"skills,omitempty"`
	Description    string            `json:"description,omitempty"`
	WebhookURL     string            `json:"webhook_url,omitempty"`
	WebhookHeaders map[string]string `json:"webhook_headers,omitempty"`
	Secret         string            `json:"secret,omitempty"`
}

// Payload is the fixed inbound request body schema.
type Payload struct {
	Action         string         `json:"action,omitempty"`
	Input          string         `json:"input"`
	RunID          string         `json:"run_id,omitempty"`
	Decision       string         `json:"decision,omitempty"`
	Comment        string         `json:"comment,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	ConversationID string         `json:"conversation_id,omitempty"`
	ExternalID     string         `json:"external_id,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// DeliveryRecord tracks an accepted inbound delivery.
type DeliveryRecord struct {
	DeliveryID     string    `json:"delivery_id"`
	ChannelID      string    `json:"channel_id"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	RunID          string    `json:"run_id"`
	ConversationID string    `json:"conversation_id,omitempty"`
	BodyHash       string    `json:"body_hash"`
	CreatedAt      time.Time `json:"created_at"`
}

// Validate checks channel id, agent_id, and secret.
func (c *Channel) Validate() error {
	if !channelIDPattern.MatchString(c.ID) {
		return fmt.Errorf("inbox channel: invalid id %q", c.ID)
	}
	if strings.TrimSpace(c.AgentID) == "" {
		return errors.New("inbox channel: agent_id required")
	}
	if strings.TrimSpace(c.Secret) == "" {
		return errors.New("inbox channel: secret required")
	}
	return nil
}

// Validate checks payload fields according to action (create_run or resume).
func (p *Payload) Validate() error {
	action := strings.TrimSpace(p.Action)
	if action == "" {
		action = ActionCreateRun
	}

	switch action {
	case ActionResume:
		if strings.TrimSpace(p.RunID) == "" {
			return errors.New("inbox payload: run_id required")
		}
		switch strings.TrimSpace(p.Decision) {
		case "approve", "reject":
		default:
			return errors.New("inbox payload: decision must be approve or reject")
		}
	case ActionCreateRun:
		input := strings.TrimSpace(p.Input)
		if input == "" {
			return errors.New("inbox payload: input required")
		}
		if len(input) > maxPayloadInputLen {
			return fmt.Errorf("inbox payload: input exceeds %d characters", maxPayloadInputLen)
		}
	default:
		return errors.New("inbox payload: unknown action")
	}

	if key := strings.TrimSpace(p.IdempotencyKey); key != "" {
		if n := len(key); n < 1 || n > maxIdempotencyKeyLen {
			return fmt.Errorf("inbox payload: idempotency_key length must be 1-%d", maxIdempotencyKeyLen)
		}
	}
	if ext := strings.TrimSpace(p.ExternalID); ext != "" {
		if n := len(ext); n < 1 || n > maxExternalIDLen {
			return fmt.Errorf("inbox payload: external_id length must be 1-%d", maxExternalIDLen)
		}
	}
	return nil
}
