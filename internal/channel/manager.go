package channel

import "context"

// ChannelSettings are the hot-updatable per-instance settings exposed via the
// generic /v0/settings/channels/{name} management plane.
type ChannelSettings struct {
	Assignee  string   `json:"assignee"`
	AgentID   string   `json:"agent_id"`
	Allowlist []string `json:"allowlist"`
	Enabled   bool     `json:"enabled"`
}

// AdapterStatus is the reconciled running state returned alongside settings.
// Reason is "" when running or intentionally disabled; "login_required" when
// credentials are missing; "start_failed" when the adapter is not polling or
// unreachable.
type AdapterStatus struct {
	Running bool   `json:"running"`
	Reason  string `json:"reason,omitempty"`
}

// LoginTicket is the QR login start response.
type LoginTicket struct {
	Ticket string `json:"ticket"`
	QRURL  string `json:"qr_url"`
}

// ManagedChannel is implemented by channels with a settings + adapter
// management plane (the webhook channel). The api layer drives it generically
// so no channel-specific handlers are needed.
type ManagedChannel interface {
	GetSettings() ChannelSettings
	UpdateSettings(ChannelSettings) AdapterStatus
	Status() AdapterStatus
	LoginStart(ctx context.Context) (LoginTicket, error)
	LoginPoll(ctx context.Context, ticket string) (status string, err error)
	Logout(ctx context.Context) error
}
