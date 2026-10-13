package store

// HostedLoginRecipe is the connector-side login form recipe for the public
// hosted login page. Persisted beside other ConnectorAuth fields.
type HostedLoginRecipe struct {
	Enabled   bool               `json:"enabled,omitempty"`
	LoginTool string             `json:"login_tool,omitempty"`
	Mode      string             `json:"mode,omitempty"` // password_form | token_paste
	Fields    []HostedLoginField `json:"fields,omitempty"`
}

// HostedLoginField maps one HTML form control to a login tool argument.
type HostedLoginField struct {
	Name  string `json:"name"`
	Label string `json:"label,omitempty"`
	Input string `json:"input,omitempty"` // text | password
	Arg   string `json:"arg"`             // tool argument key
}
