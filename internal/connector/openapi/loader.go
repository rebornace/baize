package openapi

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// ToolRoute maps an OpenAPI operation to an invokable tool.
type ToolRoute struct {
	Name        string
	OperationID string
	Description string
	Method      string
	Path        string
	InputSchema map[string]any
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
			tools = append(tools, ToolRoute{
				Name:        name,
				OperationID: op.OperationID,
				Description: desc,
				Method:      strings.ToUpper(method),
				Path:        path,
				InputSchema: mergeInputSchema(item, op),
			})
		}
	}
	return tools, nil
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

func mergeInputSchema(item *openapi3.PathItem, op *openapi3.Operation) map[string]any {
	schema := requestBodySchema(op)
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
	if schema["type"] == nil {
		schema["type"] = "object"
	}
	return schema
}

func requestBodySchema(op *openapi3.Operation) map[string]any {
	if op.RequestBody == nil || op.RequestBody.Value == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	content := op.RequestBody.Value.Content
	if content == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	mt := content.Get("application/json")
	if mt == nil || mt.Schema == nil || mt.Schema.Value == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	out := schemaToMap(mt.Schema.Value)
	if out["properties"] == nil {
		out["properties"] = map[string]any{}
	}
	return out
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
	if s.Description != "" {
		out["description"] = s.Description
	}
	return out
}
