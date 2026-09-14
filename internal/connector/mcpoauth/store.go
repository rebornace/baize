package mcpoauth

import (
	"errors"
	"time"
)

// ErrNeedsReauth indicates the access token is expired and cannot be refreshed.
var ErrNeedsReauth = errors.New("mcp oauth: re-authorization required")

// TokenBundle holds OAuth tokens in memory (sealed for persistence via SealBundle).
type TokenBundle struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}
