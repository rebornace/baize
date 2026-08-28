package inbox

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

const signaturePrefix = "v1="

var (
	ErrInvalidSignature = errors.New("inbox: invalid signature")
	ErrTimestampSkew    = errors.New("inbox: timestamp skew")
)

// Sign computes the HMAC-SHA256 v1 signature for an inbound request.
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks timestamp skew and compares the header signature to the expected HMAC.
func Verify(secret, timestamp string, body []byte, headerSig string, now time.Time, maxSkew time.Duration) error {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrInvalidSignature
	}
	requestTime := time.Unix(ts, 0)
	if now.Sub(requestTime) > maxSkew || requestTime.Sub(now) > maxSkew {
		return ErrTimestampSkew
	}
	if !strings.HasPrefix(headerSig, signaturePrefix) {
		return ErrInvalidSignature
	}
	sigHex := strings.TrimPrefix(headerSig, signaturePrefix)
	expectedSig, err := hex.DecodeString(sigHex)
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
