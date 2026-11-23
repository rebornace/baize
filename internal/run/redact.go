package run

import (
	"strings"

	"github.com/rebornace/baize/internal/identity"
)

// redactSensitiveText masks common credential shapes before an error reaches
// the run record, event trail, or chat UI. maxRunes <= 0 keeps the full text.
func redactSensitiveText(s string, maxRunes int) string {
	out := identity.RedactText(strings.TrimSpace(s))
	if maxRunes > 0 {
		out = truncateRunes(out, maxRunes)
	}
	return out
}
