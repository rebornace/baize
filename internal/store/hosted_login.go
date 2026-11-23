package store

import "strings"

// HostedLoginConfig is the connector-side hosted login configuration for the
// public login page. Persisted beside other ConnectorAuth fields.
type HostedLoginConfig struct {
	Enabled         bool                `json:"enabled,omitempty"`
	DefaultMethodID string              `json:"default_method_id,omitempty"`
	Methods         []HostedLoginMethod `json:"methods,omitempty"`
}

type HostedLoginMethod struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	Mode  string `json:"mode,omitempty"`
	// Role keys the primary MCP export identity slot (connector_id + role). Empty means ID.
	Role string `json:"role,omitempty"`
	// IdentityIDs are export identities this login method may fill (admin + user, etc.).
	IdentityIDs []string           `json:"identity_ids,omitempty"`
	LoginTool   string             `json:"login_tool,omitempty"`
	Fields      []HostedLoginField `json:"fields,omitempty"`
	OAuth       *HostedLoginOAuth  `json:"oauth,omitempty"`
}

// EffectiveRole returns Role when set, otherwise the method ID.
func (m HostedLoginMethod) EffectiveRole() string {
	if r := strings.TrimSpace(m.Role); r != "" {
		return r
	}
	return strings.TrimSpace(m.ID)
}

// PrimaryIdentityID returns the first non-empty entry in IdentityIDs.
func (m HostedLoginMethod) PrimaryIdentityID() string {
	for _, id := range m.IdentityIDs {
		if t := strings.TrimSpace(id); t != "" {
			return t
		}
	}
	return ""
}

// BindsIdentity reports whether identityID may be filled by this method.
// An empty IdentityIDs list means unrestricted (legacy / single-identity configs).
func (m HostedLoginMethod) BindsIdentity(identityID string) bool {
	identityID = strings.TrimSpace(identityID)
	if identityID == "" {
		return false
	}
	if len(m.IdentityIDs) == 0 {
		return true
	}
	for _, id := range m.IdentityIDs {
		if strings.TrimSpace(id) == identityID {
			return true
		}
	}
	return false
}

// DefaultMethod returns the method for DefaultMethodID, or the first method
// when DefaultMethodID is empty.
func (c HostedLoginConfig) DefaultMethod() (HostedLoginMethod, bool) {
	if len(c.Methods) == 0 {
		return HostedLoginMethod{}, false
	}
	id := strings.TrimSpace(c.DefaultMethodID)
	if id != "" {
		if m, ok := c.MethodByID(id); ok {
			return m, true
		}
		return HostedLoginMethod{}, false
	}
	return c.Methods[0], true
}

// MethodByID returns the method with the given id.
func (c HostedLoginConfig) MethodByID(id string) (HostedLoginMethod, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return HostedLoginMethod{}, false
	}
	for _, m := range c.Methods {
		if m.ID == id {
			return m, true
		}
	}
	return HostedLoginMethod{}, false
}

// RoleForLoginTool returns EffectiveRole for the method whose LoginTool matches
// toolName. Empty string means no match (use connector-default slot).
func (c HostedLoginConfig) RoleForLoginTool(toolName string) string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return ""
	}
	for _, m := range c.Methods {
		if strings.TrimSpace(m.LoginTool) == toolName {
			return m.EffectiveRole()
		}
	}
	return ""
}

// HostedLoginOAuth configures authorization-code login (enterprise IdP / OIDC-compatible).
type HostedLoginOAuth struct {
	AuthorizeURL string   `json:"authorize_url,omitempty"`
	TokenURL     string   `json:"token_url,omitempty"`
	ClientID     string   `json:"client_id,omitempty"`
	ClientSecret string   `json:"client_secret,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	// TokenHeader is the identity header key (default Authorization).
	TokenHeader string `json:"token_header,omitempty"`
	// TokenPrefix prepended to access_token (default "Bearer ").
	TokenPrefix string `json:"token_prefix,omitempty"`
}

// HostedLoginField maps one HTML form control to a login tool argument.
type HostedLoginField struct {
	Name  string `json:"name"`
	Label string `json:"label,omitempty"`
	Input string `json:"input,omitempty"` // text | password
	Arg   string `json:"arg"`             // tool argument key
}
