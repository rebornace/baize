// Package webhooksig provides HMAC-SHA256 request signing shared by inbound
// webhooks (inbox) and out-of-process channel adapters.
package webhooksig

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

const SignaturePrefix = "v1="

var (
	ErrInvalidSignature = errors.New("webhooksig: invalid signature")
	ErrTimestampSkew    = errors.New("webhooksig: timestamp skew")
)

// Sign computes HMAC-SHA256 over "<timestamp>."+body, returned as "v1=<hex>".
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return SignaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks timestamp skew and constant-time signature comparison.
func Verify(secret, timestamp string, body []byte, headerSig string, now time.Time, maxSkew time.Duration) error {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrInvalidSignature
	}
	requestTime := time.Unix(ts, 0)
	if now.Sub(requestTime) > maxSkew || requestTime.Sub(now) > maxSkew {
		return ErrTimestampSkew
	}
	if !strings.HasPrefix(headerSig, SignaturePrefix) {
		return ErrInvalidSignature
	}
	expectedSig, err := hex.DecodeString(strings.TrimPrefix(headerSig, SignaturePrefix))
	if err != nil {
		return ErrInvalidSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), expectedSig) {
		return ErrInvalidSignature
	}
	return nil
}
