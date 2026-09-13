package identity

import "time"

const (
	SourceEnv          = "env"
	SourceLoginCapture = "login_capture"
	SourceManual       = "manual"
)

type Identity struct {
	ID                string
	Label             string
	Scheme            string
	CredentialHeaders map[string]string
	Source            string
	Subject           string
	ClaimsSummary     map[string]any
	IsDefault         bool
	LastUsedAt        time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PublicView struct {
	ID            string         `json:"id"`
	Label         string         `json:"label"`
	Scheme        string         `json:"scheme,omitempty"`
	Source        string         `json:"source"`
	ClaimsSummary map[string]any `json:"claims_summary,omitempty"`
	IsDefault     bool           `json:"is_default"`
	LastUsedAt    *time.Time     `json:"last_used_at,omitempty"`
}
