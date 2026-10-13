package store

// HostedLoginRecipe is the connector-side login form recipe for the public
// hosted login page. Persisted beside other ConnectorAuth fields.
type HostedLoginRecipe struct {
	Enabled   bool               `json:"enabled,omitempty"`
	LoginTool string             `json:"login_tool,omitempty"`
	Mode      string             `json:"mode,omitempty"` // password_form | token_paste | basic | header_pairs | oauth
	Fields    []HostedLoginField `json:"fields,omitempty"`
	OAuth     *HostedLoginOAuth  `json:"oauth,omitempty"`
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
