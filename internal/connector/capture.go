package connector

import (
	"strings"

	"github.com/rebornace/baize/internal/identity"
)

// CaptureDefaults fills open-box login capture when config omits capture.
// default.local.yaml often only sets auth.static; without defaults, login never persists.
// To disable capture, set tool_name_glob to a non-matching pattern (e.g. "__none__").
func CaptureDefaults(c identity.CaptureConfig) identity.CaptureConfig {
	if strings.TrimSpace(c.ToolNameGlob) != "" {
		return c
	}
	c.ToolNameGlob = "*login*"
	if len(c.TokenJSONPaths) == 0 {
		c.TokenJSONPaths = []string{"accessToken", "data.accessToken", "data.token"}
	}
	if len(c.LabelJSONPaths) == 0 {
		c.LabelJSONPaths = []string{"email", "data.email"}
	}
	if strings.TrimSpace(c.HeaderTemplate) == "" {
		c.HeaderTemplate = "Bearer {{token}}"
	}
	return c
}
