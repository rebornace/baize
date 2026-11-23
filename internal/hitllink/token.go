// Package hitllink issues and verifies short-lived approval links for
// waiting_human runs. A link is a capability for one specific wait: it is
// bound to the run, the start of that wait, and an expiry. It does not carry
// tool arguments or any control-plane credential.
package hitllink

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	tokenVersion = "v1"
	tokenSep     = "."
	signPrefix   = "baize-hitl-link-v1|"
	// DefaultTTL is used when HITL waits have no expiry (approval_ttl_sec: 0).
	DefaultTTL = 24 * time.Hour
)

var (
	ErrMalformed    = errors.New("hitllink: malformed token")
	ErrExpired      = errors.New("hitllink: token expired")
	ErrBadSignature = errors.New("hitllink: bad signature")
)

// Claims is the signed content of an approval link.
type Claims struct {
	RunID        string    `json:"run_id"`
	WaitUnixNano int64     `json:"wait_unix_nano"`
	Exp          int64     `json:"exp"`
	ExpiresAt    time.Time `json:"-"`
}

type payload struct {
	RunID        string `json:"run_id"`
	WaitUnixNano int64  `json:"wait_unix_nano"`
	Exp          int64  `json:"exp"`
}

// Issue signs a link token for one run and one wait start. The expiry is
// waitedSince+ttl, so a link cannot outlive the approval wait it belongs to.
func Issue(secret []byte, runID string, waitedSince time.Time, ttl time.Duration, now time.Time) (string, time.Time, error) {
	if len(secret) == 0 {
		return "", time.Time{}, errors.New("hitllink: empty secret")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", time.Time{}, errors.New("hitllink: empty run id")
	}
	if waitedSince.IsZero() {
		return "", time.Time{}, errors.New("hitllink: zero wait start")
	}
	if ttl <= 0 {
		return "", time.Time{}, errors.New("hitllink: non-positive ttl")
	}
	exp := waitedSince.Add(ttl)
	if !exp.After(now) {
		return "", time.Time{}, errors.New("hitllink: wait already expired")
	}
	raw, err := json.Marshal(payload{
		RunID:        runID,
		WaitUnixNano: waitedSince.UTC().UnixNano(),
		Exp:          exp.Unix(),
	})
	if err != nil {
		return "", time.Time{}, err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	token := tokenVersion + tokenSep + body + tokenSep + sign(secret, body)
	return token, exp, nil
}

func sign(secret []byte, body string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signPrefix))
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify checks the token version, signature, fields and expiry. It does not
// know whether the run is still waiting; the caller must compare the returned
// wait with the current HITL state.
func Verify(secret []byte, token string, now time.Time) (Claims, error) {
	if len(secret) == 0 || strings.TrimSpace(token) == "" {
		return Claims{}, ErrMalformed
	}
	parts := strings.Split(token, tokenSep)
	if len(parts) != 3 || parts[0] != tokenVersion || parts[1] == "" || parts[2] == "" {
		return Claims{}, ErrMalformed
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return Claims{}, ErrMalformed
	}
	if strings.TrimSpace(p.RunID) == "" || p.WaitUnixNano <= 0 || p.Exp <= 0 {
		return Claims{}, ErrMalformed
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	expected := sign(secret, parts[1])
	expectedRaw, err := base64.RawURLEncoding.DecodeString(expected)
	if err != nil {
		return Claims{}, ErrMalformed
	}
	if !hmac.Equal(sig, expectedRaw) {
		return Claims{}, ErrBadSignature
	}
	exp := time.Unix(p.Exp, 0).UTC()
	if !now.Before(exp) {
		return Claims{}, ErrExpired
	}
	return Claims{
		RunID:        p.RunID,
		WaitUnixNano: p.WaitUnixNano,
		Exp:          p.Exp,
		ExpiresAt:    exp,
	}, nil
}

// FormatURL builds the public approval URL. Empty base or token returns "".
func FormatURL(publicBase, token string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	token = strings.TrimSpace(token)
	if base == "" || token == "" {
		return ""
	}
	return fmt.Sprintf("%s/v0/hitl/approvals/%s", base, token)
}
