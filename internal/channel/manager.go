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

// ProcessController is an optional extension for managed channels that own a
// controllable adapter process (autostart webhook instances). These operate on
// the OS process lifecycle, distinct from the polling start/stop driven by the
// enabled setting. Channels whose adapter is deployed independently do not
// implement it (baize does not own that process).
type ProcessController interface {
	// StartProcess launches (or adopts) the adapter process and resumes polling
	// when the channel is enabled.
	StartProcess(ctx context.Context) error
	// StopProcess stops polling and terminates the adapter process owned by
	// baize. An adopted orphan it did not spawn is left running (baize cannot
	// safely kill a process it does not own); polling is still stopped.
	StopProcess(ctx context.Context) error
	// RestartProcess terminates the owned adapter process and launches a fresh
	// one, then resumes polling when enabled. Used to recover a wedged adapter.
	RestartProcess(ctx context.Context) error
}
