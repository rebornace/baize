package hostedlogin

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/store"
)

const (
	ModePasswordForm = "password_form"
	ModeTokenPaste   = "token_paste"
	ModeBasic        = "basic"
	ModeHeaderPairs  = "header_pairs"
	ModeOAuth        = "oauth"

	draftMethodID = "draft"
)

var passwordArgRE = regexp.MustCompile(`(?i)pass|secret`)

// ValidateMethod checks one hosted-login method (per-mode completeness rules).
func ValidateMethod(m store.HostedLoginMethod) error {
	switch m.Mode {
	case "", ModePasswordForm, ModeTokenPaste, ModeBasic, ModeHeaderPairs, ModeOAuth:
	default:
		return fmt.Errorf("unsupported hosted login mode %q", m.Mode)
	}
	switch m.Mode {
	case ModePasswordForm:
		if strings.TrimSpace(m.LoginTool) == "" {
			return fmt.Errorf("password_form requires login_tool")
		}
		if !hasPasswordField(m.Fields) {
			return fmt.Errorf("password_form needs a password field")
		}
		return nil
	case ModeTokenPaste:
		if len(m.Fields) != 1 {
			return fmt.Errorf("token_paste requires exactly one field")
		}
		arg := strings.ToLower(strings.TrimSpace(m.Fields[0].Arg))
		if arg != "token" && arg != "api_key" {
			return fmt.Errorf("token_paste field arg must be token or api_key")
		}
		return nil
	case ModeBasic:
		// LoginTool optional: empty → write Authorization Basic directly.
		if len(effectiveBasicFields(m.Fields)) < 2 {
			return fmt.Errorf("basic needs username and password fields")
		}
		return nil
	case ModeHeaderPairs:
		if len(m.Fields) == 0 {
			return fmt.Errorf("header_pairs needs at least one header field")
		}
		for _, f := range m.Fields {
			if strings.TrimSpace(f.Arg) == "" && strings.TrimSpace(f.Name) == "" {
				return fmt.Errorf("header_pairs field needs name or arg (header key)")
			}
		}
		return nil
	case ModeOAuth:
		return validateOAuth(m.OAuth)
	case "":
		return fmt.Errorf("method requires mode")
	default:
		return fmt.Errorf("unsupported hosted login mode %q", m.Mode)
	}
}

// ValidateConfig checks hosted login config when enabled (methods + default_method_id).
func ValidateConfig(c store.HostedLoginConfig) error {
	if !c.Enabled {
		return nil
	}
	if len(c.Methods) == 0 {
		return fmt.Errorf("enabled hosted login requires methods")
	}
	var found bool
	for _, m := range c.Methods {
		if err := ValidateMethod(m); err != nil {
			return err
		}
		if m.ID == c.DefaultMethodID {
			found = true
		}
	}
	if strings.TrimSpace(c.DefaultMethodID) == "" || !found {
		return fmt.Errorf("default_method_id must reference a method")
	}
	return nil
}

func validateOAuth(o *store.HostedLoginOAuth) error {
	if o == nil {
		return fmt.Errorf("oauth mode requires oauth config")
	}
	if strings.TrimSpace(o.AuthorizeURL) == "" {
		return fmt.Errorf("oauth requires authorize_url")
	}
	if strings.TrimSpace(o.TokenURL) == "" {
		return fmt.Errorf("oauth requires token_url")
	}
	if strings.TrimSpace(o.ClientID) == "" {
		return fmt.Errorf("oauth requires client_id")
	}
	return nil
}

func hasPasswordField(fields []store.HostedLoginField) bool {
	for _, f := range fields {
		if strings.EqualFold(strings.TrimSpace(f.Input), "password") {
			return true
		}
		if passwordArgRE.MatchString(f.Arg) {
			return true
		}
	}
	return false
}

func effectiveBasicFields(fields []store.HostedLoginField) []store.HostedLoginField {
	if len(fields) == 0 {
		return DefaultBasicFields()
	}
	return fields
}

// DefaultBasicFields is the username/password pair used when recipe.Fields is empty.
func DefaultBasicFields() []store.HostedLoginField {
	return []store.HostedLoginField{
		{Name: "username", Label: "Username", Arg: "username", Input: "text"},
		{Name: "password", Label: "Password", Arg: "password", Input: "password"},
	}
}

// DefaultTokenFields is used when token_paste has no explicit fields.
func DefaultTokenFields() []store.HostedLoginField {
	return []store.HostedLoginField{
		{Name: "token", Label: "Token", Arg: "token", Input: "password"},
	}
}

// DefaultHeaderPairFields seeds a single Authorization header pair.
func DefaultHeaderPairFields() []store.HostedLoginField {
	return []store.HostedLoginField{
		{Name: "Authorization", Label: "Authorization", Arg: "Authorization", Input: "password"},
	}
}

// DraftRecipe builds a disabled draft config from a login tool name and its
// JSON Schema (input_schema). Prefer password_form when a password-like
// argument is present; otherwise token_paste for a single token/api_key arg.
func DraftRecipe(loginTool string, inputSchema map[string]any) store.HostedLoginConfig {
	props := schemaProperties(inputSchema)
	required := schemaRequired(inputSchema)
	keys := propertyKeys(props, required)

	fields := make([]store.HostedLoginField, 0, len(keys))
	hasPassword := false
	for _, key := range keys {
		input := "text"
		if passwordArgRE.MatchString(key) {
			input = "password"
			hasPassword = true
		}
		fields = append(fields, store.HostedLoginField{
			Name:  key,
			Label: key,
			Input: input,
			Arg:   key,
		})
	}

	method := store.HostedLoginMethod{
		ID:        draftMethodID,
		LoginTool: loginTool,
		Fields:    fields,
	}
	switch {
	case hasPassword:
		method.Mode = ModePasswordForm
	case len(fields) == 1:
		arg := strings.ToLower(fields[0].Arg)
		if arg == "token" || arg == "api_key" {
			method.Mode = ModeTokenPaste
		} else {
			method.Mode = ModePasswordForm
		}
	default:
		method.Mode = ModePasswordForm
	}
	return store.HostedLoginConfig{
		Enabled:         false,
		DefaultMethodID: draftMethodID,
		Methods:         []store.HostedLoginMethod{method},
	}
}

// DraftRecipeFromSecurity builds a disabled draft when there is no login tool
// but OpenAPI securitySchemes suggest bearer/apiKey/basic.
// schemes is a map of scheme-name → scheme object (as decoded JSON).
func DraftRecipeFromSecurity(schemes map[string]any) store.HostedLoginConfig {
	method := store.HostedLoginMethod{ID: draftMethodID}
	if len(schemes) == 0 {
		return store.HostedLoginConfig{Enabled: false}
	}
	names := make([]string, 0, len(schemes))
	for name := range schemes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw, ok := schemes[name].(map[string]any)
		if !ok {
			continue
		}
		typ := strings.ToLower(mapString(raw, "type"))
		scheme := strings.ToLower(mapString(raw, "scheme"))
		in := strings.ToLower(mapString(raw, "in"))
		switch {
		case typ == "http" && scheme == "basic":
			method.Mode = ModeBasic
			method.Fields = DefaultBasicFields()
			return draftConfigFromMethod(method)
		case typ == "http" && (scheme == "bearer" || scheme == ""):
			method.Mode = ModeTokenPaste
			method.Fields = DefaultTokenFields()
			return draftConfigFromMethod(method)
		case typ == "apikey" && (in == "header" || in == ""):
			headerName := mapString(raw, "name")
			if headerName == "" {
				headerName = "Authorization"
			}
			method.Mode = ModeHeaderPairs
			method.Fields = []store.HostedLoginField{{
				Name: headerName, Label: headerName, Arg: headerName, Input: "password",
			}}
			return draftConfigFromMethod(method)
		}
	}
	return store.HostedLoginConfig{Enabled: false}
}

func draftConfigFromMethod(m store.HostedLoginMethod) store.HostedLoginConfig {
	if m.ID == "" {
		m.ID = draftMethodID
	}
	return store.HostedLoginConfig{
		Enabled:         false,
		DefaultMethodID: m.ID,
		Methods:         []store.HostedLoginMethod{m},
	}
}

func mapString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func schemaProperties(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	raw, ok := schema["properties"]
	if !ok || raw == nil {
		return nil
	}
	switch m := raw.(type) {
	case map[string]any:
		return m
	default:
		return nil
	}
}

func schemaRequired(schema map[string]any) []string {
	if schema == nil {
		return nil
	}
	raw, ok := schema["required"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), v...)
	default:
		return nil
	}
}

func propertyKeys(props map[string]any, required []string) []string {
	if len(props) == 0 {
		return append([]string(nil), required...)
	}
	seen := make(map[string]struct{}, len(props))
	var keys []string
	for _, r := range required {
		if _, ok := props[r]; ok {
			keys = append(keys, r)
			seen[r] = struct{}{}
		}
	}
	var rest []string
	for k := range props {
		if _, ok := seen[k]; ok {
			continue
		}
		rest = append(rest, k)
	}
	sort.Strings(rest)
	return append(keys, rest...)
}
