package plugincallback

import (
	"strings"
	"time"
)

type Signer func(secret []byte, runID string, ttl time.Duration) (token string, exp time.Time, err error)

func EventURL(signer Signer, secret []byte, publicBase string, ttl time.Duration, runID string) string {
	if runID == "" || signer == nil || len(secret) == 0 || strings.TrimSpace(publicBase) == "" {
		return ""
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	token, _, err := signer(secret, runID, ttl)
	if err != nil {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	return FormatTokenURL(base, runID, token)
}
