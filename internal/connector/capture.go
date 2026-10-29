package connector

import (
	"strings"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// LoginSignalsFromAuth converts stored connector login_signals into the matcher shape.
func LoginSignalsFromAuth(a store.LoginSignalsAuth) tool.LoginSignals {
	return tool.LoginSignals{
		HTTPStatuses:           append([]int(nil), a.HTTPStatuses...),
		HTTPStatusesNeedSignal: append([]int(nil), a.HTTPStatusesNeedSignal...),
		BodyCodePath:           a.BodyCodePath,
		BodyCodes:              append([]string(nil), a.BodyCodes...),
		MessageContains:        append([]string(nil), a.MessageContains...),
	}
}

// CaptureDefaults fills open-box login capture when config omits capture.
// default.local.yaml often only sets auth.static; without defaults, login never persists.
// To disable capture, set tool_name_glob to a non-matching pattern (e.g. "__none__").
func CaptureDefaults(c identity.CaptureConfig) identity.CaptureConfig {
	if strings.EqualFold(strings.TrimSpace(c.ToolNameGlob), "__none__") {
		return c
	}
	if strings.TrimSpace(c.ToolNameGlob) == "" {
		c.ToolNameGlob = "*login*"
	}
	if len(c.TokenJSONPaths) == 0 {
		c.TokenJSONPaths = []string{
			"accessToken", "data.accessToken",
			"token", "data.token",
			"access_token", "data.access_token",
			"tokenValue", "data.tokenValue",
		}
	}
	if len(c.LabelJSONPaths) == 0 {
		c.LabelJSONPaths = []string{"email", "data.email", "data.user.email", "username", "data.username"}
	}
	if strings.TrimSpace(c.HeaderTemplate) == "" {
		c.HeaderTemplate = "Bearer {{token}}"
	}
	return c
}
