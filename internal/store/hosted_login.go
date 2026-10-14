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
	ID        string             `json:"id"`
	Label     string             `json:"label,omitempty"`
	Mode      string             `json:"mode,omitempty"`
	LoginTool string             `json:"login_tool,omitempty"`
	Fields    []HostedLoginField `json:"fields,omitempty"`
	OAuth     *HostedLoginOAuth  `json:"oauth,omitempty"`
}

// DefaultMethod returns the method for DefaultMethodID, or the first method
// when DefaultMethodID is empty.
func (c HostedLoginConfig) DefaultMethod() (HostedLoginMethod, bool) {
	if len(c.Methods) == 0 {
		return HostedLoginMethod{}, false
	}
	id := strings.TrimSpace(c.DefaultMethodID)
	if id != "" {
		for _, m := range c.Methods {
			if m.ID == id {
				return m, true
			}
		}
		return HostedLoginMethod{}, false
	}
	return c.Methods[0], true
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
