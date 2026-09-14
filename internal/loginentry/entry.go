package loginentry

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
)

type Entry struct {
	ID             string         `json:"id"`
	ConnectorID    string         `json:"connector_id"`
	ConnectorTitle string         `json:"connector_title"`
	ConnectorType  string         `json:"connector_type"`
	ToolName       string         `json:"tool_name"`
	Title          string         `json:"title"`
	LoggedIn       bool           `json:"logged_in"`
	Parameters     map[string]any `json:"parameters"`
	Required       []string       `json:"required"`
}

var sensitiveKey = regexp.MustCompile(`(?i)(password|passwd|secret|token|api_key)`)

// List returns login entries derived from connector capture config and enabled tools.
func List(st store.Store, ids identity.Store, conversationID, connectorFilter string) []Entry {
	if st == nil {
		return nil
	}

	loggedIn := sessionLoggedIn(ids, conversationID)

	connectors := st.ListConnectors()
	if connectorFilter != "" {
		var filtered []store.Connector
		for _, c := range connectors {
			if c.ID == connectorFilter {
				filtered = append(filtered, c)
				break
			}
		}
		connectors = filtered
	}

	var out []Entry
	for _, c := range connectors {
		if c.Type != "openapi" && c.Type != "http" {
			continue
		}
		capture := effectiveCapture(c)
		connectorTitle := c.ID
		for _, tool := range st.ListToolsByConnector(c.ID) {
			if !tool.Enabled {
				continue
			}
			if !identity.MatchToolName(capture.ToolNameGlob, tool.Name) {
				continue
			}
			schema := tool.InputSchema
			if schema == nil {
				schema = map[string]any{}
			}
			out = append(out, Entry{
				ID:             c.ID + "/" + tool.Name,
				ConnectorID:    c.ID,
				ConnectorTitle: connectorTitle,
				ConnectorType:  c.Type,
				ToolName:       tool.Name,
				Title:          fmt.Sprintf("登录 · %s / %s", connectorTitle, tool.Name),
				LoggedIn:       loggedIn,
				Parameters:     schema,
				Required:       RequiredFromSchema(schema),
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ConnectorID != out[j].ConnectorID {
			return out[i].ConnectorID < out[j].ConnectorID
		}
		return out[i].ToolName < out[j].ToolName
	})
	return out
}

// Find returns a single login entry by connector and tool name.
func Find(st store.Store, ids identity.Store, conversationID, connectorID, toolName string) (*Entry, bool) {
	entries := List(st, ids, conversationID, connectorID)
	for i := range entries {
		if entries[i].ToolName == toolName {
			e := entries[i]
			return &e, true
		}
	}
	return nil, false
}

// RequiredFromSchema extracts required property names from a JSON Schema object.
func RequiredFromSchema(schema map[string]any) []string {
	if len(schema) == 0 {
		return nil
	}
	raw, ok := schema["required"]
	if !ok {
		return nil
	}
	switch req := raw.(type) {
	case []any:
		out := make([]string, 0, len(req))
		for _, item := range req {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(req))
		for _, s := range req {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// ValidateArgs reports whether args satisfies all required parameter names.
func ValidateArgs(required []string, args map[string]any) error {
	if len(required) == 0 {
		return nil
	}
	for _, name := range required {
		v, ok := args[name]
		if !ok || v == nil {
			return fmt.Errorf("missing required argument %q", name)
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			return fmt.Errorf("missing required argument %q", name)
		}
	}
	return nil
}

// RedactArgs returns a copy of args with sensitive keys masked.
func RedactArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		if sensitiveKey.MatchString(k) {
			out[k] = "***"
		} else {
			out[k] = v
		}
	}
	return out
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

func sessionLoggedIn(ids identity.Store, conversationID string) bool {
	if ids == nil || conversationID == "" {
		return false
	}
	res := authresolve.OpenAPISecurityResolver{}.Resolve(context.Background(), authresolve.ResolveInput{
		Identities: ids.List(conversationID),
	})
	return res.OK && len(res.Headers) > 0
}
