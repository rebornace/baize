package mobileagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type remoteToolParam struct {
	Name        string
	In          string
	Type        string
	Description string
	Required    bool
}

type remoteTool struct {
	Name        string
	Description string
	BaseURL     string
	Path        string
	Method      string // GET, POST, PUT or PATCH; empty means GET.
	Params      []remoteToolParam
	// WholeBody is set when the request body has no expandable object schema;
	// the model then supplies the complete JSON body through the "body" arg.
	WholeBody bool
	// BodyRequired carries requestBody.required for expanded object schemas.
	BodyRequired bool
	AuthHeader   string
	AuthValue    string
}

type readonlyAPI struct {
	connectors int
	tools      []remoteTool
}

// readOnlyConnectorConfig is one connector in ConfigureReadOnlyAPIs or one
// entry of a remote config's readonly_apis array.
type readOnlyConnectorConfig struct {
	ID         string `json:"id"`
	BaseURL    string `json:"base_url"`
	AuthHeader string `json:"auth_header"`
	AuthValue  string `json:"auth_value"`
	Spec       string `json:"spec"`
	// AllowWrites opts this connector into POST/PUT/PATCH operations. It is
	// false by default, so existing configs stay GET-only.
	AllowWrites bool `json:"allow_writes"`
}

const (
	maxReadOnlyConnectors = 8
	maxReadOnlyTools      = 100
	// maxRemoteBodyBytes caps the JSON request body sent to a write endpoint.
	maxRemoteBodyBytes = 64 << 10
)

// ConfigureReadOnlyAPI replaces the on-device read-only OpenAPI tool set.
func (a *Agent) ConfigureReadOnlyAPI(baseURL, authHeader, authValue, spec string) error {
	return a.configureReadOnlyAPIs([]readOnlyConnectorConfig{{
		BaseURL:    baseURL,
		AuthHeader: authHeader,
		AuthValue:  authValue,
		Spec:       spec,
	}})
}

// ConfigureReadOnlyAPIs replaces the set with a JSON array of connectors:
//
//	[{"id":"tickets","base_url":"https://…","auth_header":"Authorization",
//	  "auth_value":"Bearer …","spec":"openapi: 3.0.3 …",
//	  "allow_writes": false}]
//
// GET is always parsed. POST/PUT/PATCH are parsed only when a connector sets
// allow_writes; every call still pauses for foreground approval. Tool names
// must be unique across all connectors and the built-ins.
func (a *Agent) ConfigureReadOnlyAPIs(configsJSON string) error {
	var cfgs []readOnlyConnectorConfig
	if err := json.Unmarshal([]byte(strings.TrimSpace(configsJSON)), &cfgs); err != nil {
		return fmt.Errorf("readonly api: invalid connectors json: %w", err)
	}
	return a.configureReadOnlyAPIs(cfgs)
}

func (a *Agent) configureReadOnlyAPIs(cfgs []readOnlyConnectorConfig) error {
	cfg, err := parseReadOnlyAPIs(cfgs)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.readOnly = cfg
	if a.pending != nil && a.pending.Remote != nil {
		a.pending = nil
	}
	a.mu.Unlock()
	return nil
}

// ClearReadOnlyAPI removes the connector tool set and any pending remote call.
func (a *Agent) ClearReadOnlyAPI() {
	a.mu.Lock()
	a.readOnly = nil
	if a.pending != nil && a.pending.Remote != nil {
		a.pending = nil
	}
	a.mu.Unlock()
}

// RemoteToolCount returns the number of configured connector tools.
func (a *Agent) RemoteToolCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.readOnly == nil {
		return 0
	}
	return len(a.readOnly.tools)
}

// RemoteWriteToolCount returns how many connector tools use POST/PUT/PATCH.
func (a *Agent) RemoteWriteToolCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return writeToolCount(a.readOnly)
}

// RemoteToolNames returns comma-separated connector tool names (empty if none).
func (a *Agent) RemoteToolNames() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.readOnly == nil {
		return ""
	}
	names := make([]string, 0, len(a.readOnly.tools))
	for _, tool := range a.readOnly.tools {
		names = append(names, tool.Name)
	}
	return strings.Join(names, ", ")
}

// ConfigureReadOnlyAPI replaces the default session's GET-only connector.
func ConfigureReadOnlyAPI(baseURL, authHeader, authValue, spec string) error {
	return defaultSession.ConfigureReadOnlyAPI(baseURL, authHeader, authValue, spec)
}

// ConfigureReadOnlyAPIs replaces the default session's set with a JSON array
// of connectors (see Agent.ConfigureReadOnlyAPIs).
func ConfigureReadOnlyAPIs(configsJSON string) error {
	return defaultSession.ConfigureReadOnlyAPIs(configsJSON)
}

// ClearReadOnlyAPI removes the default session's connector tool set.
func ClearReadOnlyAPI() { defaultSession.ClearReadOnlyAPI() }

// RemoteToolCount returns the default session's connector tool count.
func RemoteToolCount() int { return defaultSession.RemoteToolCount() }

// RemoteWriteToolCount returns the default session's write tool count.
func RemoteWriteToolCount() int { return defaultSession.RemoteWriteToolCount() }

// RemoteToolNames returns the default session's comma-separated tool names.
func RemoteToolNames() string { return defaultSession.RemoteToolNames() }

func (a *Agent) remoteToolLocked(name string) *remoteTool {
	if a.readOnly == nil {
		return nil
	}
	for i := range a.readOnly.tools {
		if a.readOnly.tools[i].Name == name {
			tool := a.readOnly.tools[i]
			return &tool
		}
	}
	return nil
}

func parseReadOnlyAPIs(cfgs []readOnlyConnectorConfig) (*readonlyAPI, error) {
	if len(cfgs) == 0 {
		return nil, fmt.Errorf("readonly api: at least one connector is required")
	}
	if len(cfgs) > maxReadOnlyConnectors {
		return nil, fmt.Errorf("readonly api: at most %d connectors are allowed", maxReadOnlyConnectors)
	}
	// owner map: tool name -> connector label, so cross-connector collisions
	// name both sides. Built-ins are reserved everywhere.
	owners := map[string]string{"get_time": "built-in", "http_get": "built-in"}
	var tools []remoteTool
	for i, cfg := range cfgs {
		label := strings.TrimSpace(cfg.ID)
		if label == "" {
			label = fmt.Sprintf("connector %d", i+1)
		}
		connTools, err := parseReadOnlyConnector(cfg)
		if err != nil {
			return nil, err
		}
		for _, tool := range connTools {
			if prev, ok := owners[tool.Name]; ok {
				if prev == "built-in" {
					return nil, fmt.Errorf("readonly api: reserved tool name %q", tool.Name)
				}
				return nil, fmt.Errorf("readonly api: duplicate tool name %q (%s and %s)", tool.Name, prev, label)
			}
			owners[tool.Name] = label
			tools = append(tools, tool)
		}
	}
	if len(tools) == 0 {
		return nil, fmt.Errorf("readonly api: no GET tools")
	}
	if len(tools) > maxReadOnlyTools {
		return nil, fmt.Errorf("readonly api: at most %d tools are allowed", maxReadOnlyTools)
	}
	return &readonlyAPI{connectors: len(cfgs), tools: tools}, nil
}

// parseReadOnlyConnector extracts the supported tools of one connector. GET is
// always parsed; POST/PUT/PATCH only when the connector sets allow_writes.
// DELETE is deliberately unsupported. Name uniqueness is enforced by
// parseReadOnlyAPIs across the whole set.
func parseReadOnlyConnector(cfg readOnlyConnectorConfig) ([]remoteTool, error) {
	authHeader := strings.TrimSpace(cfg.AuthHeader)
	authValue := strings.TrimSpace(cfg.AuthValue)
	if (authHeader == "") != (authValue == "") {
		return nil, fmt.Errorf("readonly api: auth_header and auth_value must both be set or both empty")
	}
	root, err := decodeReadOnlySpec(cfg.Spec)
	if err != nil {
		return nil, err
	}
	if v := mapString(root, "openapi"); !strings.HasPrefix(v, "3.") {
		return nil, fmt.Errorf("readonly api: OpenAPI 3 spec required")
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = readOnlyServerURL(root)
	}
	base, err := normalizeReadOnlyBase(baseURL)
	if err != nil {
		return nil, err
	}
	paths, ok := root["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		return nil, fmt.Errorf("readonly api: no GET tools")
	}
	keys := make([]string, 0, len(paths))
	for path := range paths {
		keys = append(keys, path)
	}
	sort.Strings(keys)

	methods := []string{"get"}
	if cfg.AllowWrites {
		methods = append(methods, "post", "put", "patch")
	}
	var tools []remoteTool
	for _, path := range keys {
		item, ok := paths[path].(map[string]any)
		if !ok {
			continue
		}
		for _, method := range methods {
			op, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			name := mapString(op, "operationId")
			if name == "" {
				name = method + "_" + sanitizeToolName(path)
			}
			name = sanitizeToolName(name)
			if name == "" {
				return nil, fmt.Errorf("readonly api: empty tool name for %s %s", strings.ToUpper(method), path)
			}

			params := collectReadOnlyParams(item, op)
			wholeBody, bodyRequired := false, false
			if method != "get" {
				bodyParams, whole, required := collectRequestBodyParams(root, op)
				merged, err := mergeToolParams(params, bodyParams)
				if err != nil {
					return nil, fmt.Errorf("readonly api: %s %s: %w", strings.ToUpper(method), path, err)
				}
				params = merged
				wholeBody, bodyRequired = whole, required
			}
			tools = append(tools, remoteTool{
				Name:         name,
				Description:  readOnlyDescription(op, method, path),
				BaseURL:      base,
				Path:         path,
				Method:       strings.ToUpper(method),
				Params:       params,
				WholeBody:    wholeBody,
				BodyRequired: bodyRequired,
				AuthHeader:   authHeader,
				AuthValue:    authValue,
			})
		}
	}
	return tools, nil
}

// mergeToolParams appends body params and rejects a body field whose name is
// already used by a path/query parameter, so the tool schema stays unambiguous.
func mergeToolParams(base, extra []remoteToolParam) ([]remoteToolParam, error) {
	seen := make(map[string]bool, len(base)+len(extra))
	for _, p := range base {
		seen[p.Name] = true
	}
	for _, p := range extra {
		if seen[p.Name] {
			return nil, fmt.Errorf("body field %q conflicts with a path/query parameter", p.Name)
		}
		seen[p.Name] = true
		base = append(base, p)
	}
	return base, nil
}

// collectRequestBodyParams expands an application/json request body into body
// params. When the schema is not an object with properties, the caller gets a
// single "body" param plus whole=true and the raw JSON value is sent as-is.
func collectRequestBodyParams(root, op map[string]any) ([]remoteToolParam, bool, bool) {
	rawBody, _ := op["requestBody"].(map[string]any)
	rawBody = resolveLocalRef(root, rawBody)
	if rawBody == nil {
		return nil, false, false
	}
	required, _ := rawBody["required"].(bool)
	content, _ := rawBody["content"].(map[string]any)
	media, _ := content["application/json"].(map[string]any)
	if media == nil {
		// Unsupported body media type: the tool is still callable without a body.
		return nil, false, false
	}
	schema, _ := media["schema"].(map[string]any)
	schema = resolveLocalRef(root, schema)

	if props := schemaProperties(root, schema); len(props) > 0 {
		requiredNames := map[string]bool{}
		if raw, ok := schema["required"].([]any); ok {
			for _, name := range raw {
				if s, ok := name.(string); ok {
					requiredNames[s] = true
				}
			}
		}
		names := make([]string, 0, len(props))
		for name := range props {
			names = append(names, name)
		}
		sort.Strings(names)
		params := make([]remoteToolParam, 0, len(names))
		for _, name := range names {
			prop, _ := props[name].(map[string]any)
			if resolved := resolveLocalRef(root, prop); resolved != nil {
				prop = resolved
			}
			params = append(params, remoteToolParam{
				Name:        name,
				In:          "body",
				Type:        schemaType(prop),
				Description: firstNonEmpty(mapString(prop, "description"), mapString(prop, "title")),
				Required:    requiredNames[name],
			})
		}
		return params, false, required
	}

	typ := "object"
	if schema != nil {
		typ = schemaType(schema)
	}
	return []remoteToolParam{{
		Name:        "body",
		In:          "body",
		Type:        typ,
		Description: "Complete JSON request body",
		Required:    required,
	}}, true, required
}

// schemaProperties resolves object properties, including one level of $ref on
// individual property schemas (nested objects stay opaque).
func schemaProperties(root, schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	resolved := resolveLocalRef(root, schema)
	if resolved == nil {
		return nil
	}
	props, _ := resolved["properties"].(map[string]any)
	return props
}

// resolveLocalRef follows local #/… $refs (for example
// #/components/schemas/CreateTicket) up to a small depth. External refs and
// cycles are treated as unresolved.
func resolveLocalRef(root map[string]any, node map[string]any) map[string]any {
	if node == nil {
		return nil
	}
	for i := 0; i < 8; i++ {
		ref := mapString(node, "$ref")
		if ref == "" {
			return node
		}
		next, ok := lookupLocalRef(root, ref)
		if !ok {
			return nil
		}
		node = next
	}
	return nil
}

func lookupLocalRef(root map[string]any, ref string) (map[string]any, bool) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, false
	}
	var cur any = root
	for _, raw := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		part := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur = m[part]
	}
	m, ok := cur.(map[string]any)
	return m, ok
}

func schemaType(schema map[string]any) string {
	if schema == nil {
		return "string"
	}
	if t := mapString(schema, "type"); t != "" {
		return t
	}
	if _, ok := schema["properties"]; ok {
		return "object"
	}
	if _, ok := schema["items"]; ok {
		return "array"
	}
	return "string"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func decodeReadOnlySpec(spec string) (map[string]any, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("readonly api: empty spec")
	}
	var root map[string]any
	if err := yaml.Unmarshal([]byte(spec), &root); err != nil {
		return nil, fmt.Errorf("readonly api: invalid spec: %w", err)
	}
	if root == nil {
		return nil, fmt.Errorf("readonly api: invalid spec")
	}
	return root, nil
}

func mapString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func readOnlyServerURL(root map[string]any) string {
	servers, _ := root["servers"].([]any)
	if len(servers) == 0 {
		return ""
	}
	first, _ := servers[0].(map[string]any)
	return mapString(first, "url")
}

func normalizeReadOnlyBase(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("readonly api: base_url is required (or set servers[0].url)")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("readonly api: base_url must be absolute http(s)")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("readonly api: base_url must use http or https")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func collectReadOnlyParams(item, op map[string]any) []remoteToolParam {
	var out []remoteToolParam
	seen := map[string]bool{}
	for _, source := range []map[string]any{item, op} {
		raw, _ := source["parameters"].([]any)
		for _, p := range raw {
			param, ok := p.(map[string]any)
			if !ok || param["$ref"] != nil {
				continue
			}
			in := strings.ToLower(mapString(param, "in"))
			if in != "path" && in != "query" {
				continue
			}
			name := mapString(param, "name")
			if name == "" || seen[in+"\x00"+name] {
				continue
			}
			seen[in+"\x00"+name] = true
			typ := "string"
			if schema, ok := param["schema"].(map[string]any); ok {
				if v := mapString(schema, "type"); v != "" {
					typ = v
				}
			}
			required, _ := param["required"].(bool)
			if in == "path" {
				required = true
			}
			out = append(out, remoteToolParam{
				Name:        name,
				In:          in,
				Type:        typ,
				Description: mapString(param, "description"),
				Required:    required,
			})
		}
	}
	return out
}

func readOnlyDescription(op map[string]any, method, path string) string {
	summary := mapString(op, "summary")
	desc := mapString(op, "description")
	switch {
	case summary != "" && desc != "" && summary != desc:
		return summary + " — " + desc
	case summary != "":
		return summary
	case desc != "":
		return desc
	default:
		return strings.ToUpper(method) + " " + path
	}
}

func sanitizeToolName(raw string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range raw {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
		} else if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_-")
}

func (t remoteTool) spec() toolSpec {
	var spec toolSpec
	spec.Type = "function"
	spec.Function.Name = t.Name
	spec.Function.Description = t.Description
	props := map[string]any{}
	var required []string
	for _, p := range t.Params {
		prop := map[string]any{"type": p.Type}
		if p.Description != "" {
			prop["description"] = p.Description
		}
		props[p.Name] = prop
		if p.Required {
			required = append(required, p.Name)
		}
	}
	spec.Function.Parameters = map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		spec.Function.Parameters["required"] = required
	}
	return spec
}

func (t remoteTool) buildURL(argsJSON string) (string, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	path := t.Path
	for _, p := range t.Params {
		if p.In != "path" {
			continue
		}
		value, ok := args[p.Name]
		if !ok || value == nil || strings.TrimSpace(fmt.Sprint(value)) == "" {
			return "", fmt.Errorf("missing path parameter %s", p.Name)
		}
		path = strings.ReplaceAll(path, "{"+p.Name+"}", url.PathEscape(fmt.Sprint(value)))
	}
	if strings.Contains(path, "{") {
		return "", fmt.Errorf("unresolved path parameter in %s", path)
	}
	u, err := url.Parse(strings.TrimRight(t.BaseURL, "/") + path)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid request URL")
	}
	q := u.Query()
	for _, p := range t.Params {
		if p.In != "query" {
			continue
		}
		value, ok := args[p.Name]
		if !ok || value == nil {
			if p.Required {
				return "", fmt.Errorf("missing query parameter %s", p.Name)
			}
			continue
		}
		if s := strings.TrimSpace(fmt.Sprint(value)); s != "" {
			q.Set(p.Name, s)
		} else if p.Required {
			return "", fmt.Errorf("missing query parameter %s", p.Name)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (t remoteTool) method() string {
	if t.Method == "" {
		return http.MethodGet
	}
	return strings.ToUpper(t.Method)
}

func (t remoteTool) isWrite() bool {
	return t.method() != http.MethodGet
}

func (t remoteTool) summary(argsJSON string) string {
	var args map[string]any
	_ = json.Unmarshal([]byte(argsJSON), &args)
	path := t.Path
	for _, p := range t.Params {
		if p.In != "path" {
			continue
		}
		if value, ok := args[p.Name]; ok {
			path = strings.ReplaceAll(path, "{"+p.Name+"}", url.PathEscape(fmt.Sprint(value)))
		}
	}
	q := url.Values{}
	for _, p := range t.Params {
		if p.In != "query" {
			continue
		}
		if value, ok := args[p.Name]; ok {
			if s := strings.TrimSpace(fmt.Sprint(value)); s != "" {
				q.Set(p.Name, s)
			}
		}
	}
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	line := t.method() + " " + path
	if body, err := t.bodyPayload(args, false); err == nil && len(body) > 0 {
		line += " " + string(body)
	}
	return truncate(line, 200)
}

// bodyPayload builds the JSON request body. requireAll enforces required body
// fields (execution); summary generation passes false so partial arguments are
// still previewable.
func (t remoteTool) bodyPayload(args map[string]any, requireAll bool) ([]byte, error) {
	if !t.isWrite() {
		return nil, nil
	}
	if t.WholeBody {
		raw, ok := args["body"]
		if !ok || raw == nil {
			if requireAll && t.bodyParamRequired() {
				return nil, fmt.Errorf("missing request body")
			}
			return nil, nil
		}
		body, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid request body: %w", err)
		}
		return checkRemoteBodySize(body)
	}
	fields := map[string]any{}
	for _, p := range t.Params {
		if p.In != "body" {
			continue
		}
		value, ok := args[p.Name]
		if !ok || value == nil {
			if requireAll && p.Required {
				return nil, fmt.Errorf("missing body field %s", p.Name)
			}
			continue
		}
		fields[p.Name] = value
	}
	if len(fields) == 0 {
		if requireAll && t.BodyRequired {
			return []byte("{}"), nil
		}
		return nil, nil
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}
	return checkRemoteBodySize(body)
}

func (t remoteTool) bodyPayloadJSON(argsJSON string, requireAll bool) ([]byte, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	return t.bodyPayload(args, requireAll)
}

func (t remoteTool) bodyParamRequired() bool {
	for _, p := range t.Params {
		if p.In == "body" && p.Name == "body" {
			return p.Required
		}
	}
	return false
}

func checkRemoteBodySize(body []byte) ([]byte, error) {
	if len(body) > maxRemoteBodyBytes {
		return nil, fmt.Errorf("request body exceeds %d bytes", maxRemoteBodyBytes)
	}
	return body, nil
}

func (a *Agent) execRemoteTool(tool *remoteTool, argsJSON string) (string, error) {
	built, err := tool.buildURL(argsJSON)
	if err != nil {
		return "", err
	}
	payload, err := tool.bodyPayloadJSON(argsJSON, true)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(built)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), httpGetTimeout)
	defer cancel()
	var reader io.Reader
	if len(payload) > 0 {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, tool.method(), u.String(), reader)
	if err != nil {
		return "", err
	}
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if tool.AuthHeader != "" {
		req.Header.Set(tool.AuthHeader, tool.AuthValue)
	}
	client := a.fetchClient
	if client == nil {
		client = &http.Client{Timeout: httpGetTimeout, CheckRedirect: limitRedirects}
	}
	if tool.isWrite() {
		// A redirected write could replay a body or leak the auth header to a
		// different host; return the 3xx response instead of following it.
		cloned := *client
		cloned.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		client = &cloned
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpGetMaxBytes+1))
	if err != nil {
		return "", err
	}
	truncated := false
	if len(body) > httpGetMaxBytes {
		body = body[:httpGetMaxBytes]
		truncated = true
	}
	out, _ := json.Marshal(map[string]any{
		"status":    resp.StatusCode,
		"method":    tool.method(),
		"truncated": truncated,
		"body":      string(body),
		"final_url": resp.Request.URL.String(),
	})
	return string(out), nil
}

// writeToolCount reports how many tools use a non-GET method.
func writeToolCount(api *readonlyAPI) int {
	if api == nil {
		return 0
	}
	count := 0
	for _, tool := range api.tools {
		if tool.isWrite() {
			count++
		}
	}
	return count
}
