// Package plugincallback issues and verifies short-lived HMAC tokens used to
// authenticate sidecar plugin callbacks addressed to a specific Run.
package plugincallback

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// tokenSep separates the exp payload from the signature inside a token.
// '.' is URL-safe (RFC 3986 unreserved) so the token can travel in a query
// string without percent-encoding beyond standard query handling.
const tokenSep = "."

// msgSep separates runID and expUnix inside the signed HMAC message.
const msgSep = "|"

// Errors returned by Verify. Comparison is by errors.Is / direct equality.
var (
	ErrMalformedToken = errors.New("plugincallback: malformed token")
	ErrTokenExpired   = errors.New("plugincallback: token expired")
	ErrBadSignature   = errors.New("plugincallback: bad signature")
)

// Issue signs a short-lived token binding the given runID with an expiry
// computed as now+ttl. It returns the token string and the absolute expiry.
// The secret must be non-empty; callers (bootstrap) are responsible for
// generating an ephemeral secret when none is configured.
func Issue(secret []byte, runID string, ttl time.Duration) (string, time.Time, error) {
	if len(secret) == 0 {
		return "", time.Time{}, errors.New("plugincallback: empty secret")
	}
	if runID == "" {
		return "", time.Time{}, errors.New("plugincallback: empty run id")
	}
	if ttl <= 0 {
		return "", time.Time{}, errors.New("plugincallback: non-positive ttl")
	}
	exp := time.Now().Add(ttl)
	expUnix := exp.Unix()
	token, err := sign(secret, runID, expUnix)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, exp, nil
}

// sign builds the canonical token for a given expUnix. Exported via lowercase
// to keep the package API minimal; tests reach in through Verify.
func sign(secret []byte, runID string, expUnix int64) (string, error) {
	expStr := strconv.FormatInt(expUnix, 10)
	msg := runID + msgSep + expStr
	mac := hmac.New(sha256.New, secret)
	if _, err := mac.Write([]byte(msg)); err != nil {
		return "", err
	}
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	payload := base64.RawURLEncoding.EncodeToString([]byte(expStr))
	return payload + tokenSep + sig, nil
}

// Verify validates that token was issued for runID with the same secret and
// has not expired as of now. It recomputes the HMAC over the runID and the
// exp embedded in the token, so a token bound to runA cannot validate runB
// even if presented with runB's path.
func Verify(secret []byte, runID, token string, now time.Time) error {
	if len(secret) == 0 {
		return errors.New("plugincallback: empty secret")
	}
	if runID == "" {
		return errors.New("plugincallback: empty run id")
	}
	payload, sig, ok := strings.Cut(token, tokenSep)
	if !ok {
		return ErrMalformedToken
	}
	expBytes, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return ErrMalformedToken
	}
	expUnix, err := strconv.ParseInt(string(expBytes), 10, 64)
	if err != nil {
		return ErrMalformedToken
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return ErrMalformedToken
	}
	if now.Unix() >= expUnix {
		return ErrTokenExpired
	}
	msg := runID + msgSep + strconv.FormatInt(expUnix, 10)
	mac := hmac.New(sha256.New, secret)
	if _, err := mac.Write([]byte(msg)); err != nil {
		return err
	}
	if !hmac.Equal(mac.Sum(nil), sigBytes) {
		return ErrBadSignature
	}
	return nil
}

// FormatTokenURL builds the callback URL the Runtime injects into sidecar
// invoke context. publicBase must already be normalized (no trailing slash).
func FormatTokenURL(publicBase, runID, token string) string {
	return fmt.Sprintf("%s/v0/runs/%s/plugin-callbacks?token=%s", publicBase, runID, token)
}
