package inbox

import (
	"errors"
	"time"

	"github.com/rebornace/baize/internal/webhooksig"
)

const signaturePrefix = webhooksig.SignaturePrefix

var (
	ErrInvalidSignature = webhooksig.ErrInvalidSignature
	ErrTimestampSkew    = webhooksig.ErrTimestampSkew
)

// Sign delegates to the shared webhooksig package.
func Sign(secret, timestamp string, body []byte) string {
	return webhooksig.Sign(secret, timestamp, body)
}

// Verify delegates to the shared webhooksig package.
func Verify(secret, timestamp string, body []byte, headerSig string, now time.Time, maxSkew time.Duration) error {
	if err := webhooksig.Verify(secret, timestamp, body, headerSig, now, maxSkew); err != nil {
		if errors.Is(err, webhooksig.ErrTimestampSkew) {
			return ErrTimestampSkew
		}
		return ErrInvalidSignature
	}
	return nil
}
