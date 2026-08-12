package openapi

import (
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// bodyArrayKey is the wrapper property when an OpenAPI request body is a JSON array.
// LLM tool schemas must be type=object; invoke unwraps this key back to a JSON array.
const bodyArrayKey = "items"

// ToolRoute maps an OpenAPI operation to an invokable tool.
type ToolRoute struct {
	Name        string
	OperationID string
	Description string
	Method      string
	Path        string
	InputSchema map[string]any
	// BodyKind: ""|"object" (default JSON object body), "array" (send args[bodyArrayKey] as JSON array),
	// "value" (send args["value"] as scalar/json body).
	BodyKind string
	// Security lists OpenAPI security scheme names required by the operation
	// (operation-level security overrides document-level when present).
	Security []string
}

// LoadTools parses an OpenAPI 3 spec file and returns one ToolRoute per operation.
func LoadTools(specPath string) ([]ToolRoute, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("load openapi: %w", err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		return nil, fmt.Errorf("validate openapi: %w", err)
	}

	var tools []ToolRoute
	if doc.Paths == nil {
		return tools, nil
	}

	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for method, op := range item.Operations() {
			if op == nil {
				continue
			}
			name := op.OperationID
			if name == "" {
				name = normalizeOpName(method, path)
			}
			desc := op.Description
			if desc == "" {
				desc = op.Summary
			}
			schema, bodyKind := mergeInputSchema(item, op)
			tools = append(tools, ToolRoute{
				Name:        name,
				OperationID: op.OperationID,
				Description: desc,
				Method:      strings.ToUpper(method),
				Path:        path,
				InputSchema: schema,
				BodyKind:    bodyKind,
				Security:    operationSecurity(doc, op),
			})
		}
	}
	return tools, nil
}

// operationSecurity returns scheme names from operation security, falling back
// to document-level security. A non-nil empty operation security clears global.
func operationSecurity(doc *openapi3.T, op *openapi3.Operation) []string {
	var reqs openapi3.SecurityRequirements
	if op.Security != nil {
		reqs = *op.Security
	} else if doc != nil {
		reqs = doc.Security
	}
	return schemeNames(reqs)
}

func schemeNames(reqs openapi3.SecurityRequirements) []string {
	if len(reqs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, req := range reqs {
		var names []string
		for name := range req {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
		sort.Strings(names)
		out = append(out, names...)
	}
	return out
}

func normalizeOpName(method, path string) string {
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	var cleaned []string
	for _, p := range parts {
		p = strings.Trim(p, "{}")
		if p == "" {
			continue
		}
		cleaned = append(cleaned, p)
	}
	base := strings.Join(cleaned, "_")
	if base == "" {
		base = "root"
	}
	return strings.ToLower(method) + "_" + base
}

func mergeInputSchema(item *openapi3.PathItem, op *openapi3.Operation) (map[string]any, string) {
	body := requestBodySchema(op)
	bodyKind := "object"
	var schema map[string]any

	switch typ, _ := body["type"].(string); typ {
	case "array":
		// OpenAI-compatible tool calling requires parameters.type == object.
		schema = map[string]any{
			"type": "object",
			"properties": map[string]any{
				bodyArrayKey: body,
			},
			"required": []any{bodyArrayKey},
		}
		bodyKind = "array"
	case "object", "":
		if body["properties"] == nil {
			body["properties"] = map[string]any{}
		}
		body["type"] = "object"
		schema = body
		bodyKind = "object"
	default:
		schema = map[string]any{
			"type": "object",
			"properties": map[string]any{
				"value": body,
			},
			"required": []any{"value"},
		}
		bodyKind = "value"
	}

	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
		schema["properties"] = props
	}
	required, _ := schema["required"].([]any)
	requiredSet := map[string]bool{}
	for _, r := range required {
		if s, ok := r.(string); ok {
			requiredSet[s] = true
		}
	}

	addPathParams := func(params openapi3.Parameters) {
		for _, ref := range params {
			if ref == nil || ref.Value == nil {
				continue
			}
			p := ref.Value
			if p.In != openapi3.ParameterInPath {
				continue
			}
			prop := map[string]any{"type": "string"}
			if p.Schema != nil && p.Schema.Value != nil {
				prop = schemaToMap(p.Schema.Value)
			}
			props[p.Name] = prop
			if p.Required {
				requiredSet[p.Name] = true
			}
		}
	}
	if item != nil {
		addPathParams(item.Parameters)
	}
	addPathParams(op.Parameters)

	if len(requiredSet) > 0 {
		req := make([]any, 0, len(requiredSet))
		for name := range requiredSet {
			req = append(req, name)
		}
		schema["required"] = req
	}
	schema["type"] = "object"
	return schema, bodyKind
}

func requestBodySchema(op *openapi3.Operation) map[string]any {
	emptyObject := func() map[string]any {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	if op.RequestBody == nil || op.RequestBody.Value == nil {
		return emptyObject()
	}
	content := op.RequestBody.Value.Content
	if content == nil {
		return emptyObject()
	}
	mt := content.Get("application/json")
	if mt == nil || mt.Schema == nil || mt.Schema.Value == nil {
		return emptyObject()
	}
	return schemaToMap(mt.Schema.Value)
}

func schemaToMap(s *openapi3.Schema) map[string]any {
	out := map[string]any{}
	if s.Type != nil {
		if t := s.Type.Slice(); len(t) > 0 {
			out["type"] = t[0]
		}
	}
	if len(s.Required) > 0 {
		req := make([]any, len(s.Required))
		for i, r := range s.Required {
			req[i] = r
		}
		out["required"] = req
	}
	if len(s.Properties) > 0 {
		props := map[string]any{}
		for name, ref := range s.Properties {
			if ref == nil || ref.Value == nil {
				continue
			}
			props[name] = schemaToMap(ref.Value)
		}
		out["properties"] = props
	}
	if s.Items != nil && s.Items.Value != nil {
		out["items"] = schemaToMap(s.Items.Value)
	}
	if s.Description != "" {
		out["description"] = s.Description
	}
	return out
}
