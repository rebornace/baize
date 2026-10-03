package loginmanage

import (
	"regexp"
	"sort"

	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
)

// CompanionGlobs are fixed name patterns (spec §5.2); case-insensitive via MatchToolName.
var CompanionGlobs = []string{
	"*sms*",
	"*otp*",
	"*verify*",
	"*oauth*",
	"*token*",
	"*auth*",
}

const logoutGlob = "*logout*"

var connectorIDSafe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// NormalizeConnectorID keeps only [a-zA-Z0-9_-] for skill paths and ids.
func NormalizeConnectorID(id string) string {
	return connectorIDSafe.ReplaceAllString(id, "")
}

// SkillID returns managed login skill id login-<normalized>, or "" if normalization is empty.
func SkillID(connectorID string) string {
	n := NormalizeConnectorID(connectorID)
	if n == "" {
		return ""
	}
	return "login-" + n
}

// SelectTools returns enabled tool names matching capture glob or companion patterns, excluding logout.
func SelectTools(st store.Store, connectorID string) []string {
	if st == nil {
		return nil
	}
	c, err := st.GetConnector(connectorID)
	if err != nil {
		return nil
	}
	capture := effectiveCapture(c)
	var out []string
	for _, tool := range st.ListToolsByConnector(connectorID) {
		if !tool.Enabled {
			continue
		}
		if identity.MatchToolName(logoutGlob, tool.Name) {
			continue
		}
		if toolMatchesLoginSelection(capture.ToolNameGlob, tool.Name) {
			out = append(out, tool.Name)
		}
	}
	sort.Strings(out)
	return out
}

func toolMatchesLoginSelection(captureGlob, toolName string) bool {
	if identity.MatchToolName(captureGlob, toolName) {
		return true
	}
	for _, g := range CompanionGlobs {
		if identity.MatchToolName(g, toolName) {
			return true
		}
	}
	return false
}

func effectiveCapture(c store.Connector) identity.CaptureConfig {
	cap := identity.CaptureConfig{
		ToolNameGlob:   c.Auth.Capture.ToolNameGlob,
		TokenJSONPaths: append([]string(nil), c.Auth.Capture.TokenJSONPaths...),
		LabelJSONPaths: append([]string(nil), c.Auth.Capture.LabelJSONPaths...),
		HeaderTemplate: c.Auth.Capture.HeaderTemplate,
		DefaultScheme:  c.Auth.Capture.DefaultScheme,
	}
	return connector.CaptureDefaults(cap)
}
