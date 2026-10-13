package mcpexport

import (
	"fmt"
	"strings"
)

const authExpiredMessageTemplate = "Session for %s expired. Do not retry this or other Baize tools. Ask the user to re-login in Baize, then continue."

var authFailureCodes = map[string]struct{}{
	"login_required":        {},
	"oauth_reauth_required": {},
}

var authFailureErrSubstrings = []string{
	"401",
	"403",
	"unauthorized",
	"forbidden",
}

// AuthExpiredMessage returns the instructional MCP export auth expiry text.
func AuthExpiredMessage(identityName string) string {
	return AuthExpiredMessageWithURL(identityName, "")
}

// AuthExpiredMessageWithURL returns expiry instructions. When hostedURL is set,
// the user is directed to the public hosted login page instead of (only) the
// Baize settings UI.
func AuthExpiredMessageWithURL(identityName, hostedURL string) string {
	name := strings.TrimSpace(identityName)
	if name == "" {
		name = "export identity"
	}
	base := fmt.Sprintf(
		"Session for %s expired. Do not retry this or other Baize tools.",
		name,
	)
	hostedURL = strings.TrimSpace(hostedURL)
	if hostedURL != "" {
		return base +
			" Open this URL in a browser to sign in (do not paste passwords into chat): " +
			hostedURL +
			" Then continue. Intranet admins may also update the export identity in Baize settings."
	}
	return strings.Replace(authExpiredMessageTemplate, "%s", name, 1)
}

// IsAuthFailure reports whether a tool invoke result indicates expired or missing auth.
func IsAuthFailure(content map[string]any, isError bool, err error) bool {
	if err != nil {
		msg := strings.ToLower(err.Error())
		for _, sub := range authFailureErrSubstrings {
			if strings.Contains(msg, sub) {
				return true
			}
		}
	}
	if !isError || content == nil {
		return false
	}
	code, _ := content["code"].(string)
	code = strings.TrimSpace(strings.ToLower(code))
	_, ok := authFailureCodes[code]
	return ok
}
