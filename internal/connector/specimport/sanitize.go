package specimport

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

var rePathTemplate = regexp.MustCompile(`\{([^/}]+)\}`)

// sanitizeDoc repairs common real-world Swagger quirks that survive conversion
// and fail kin-openapi Validate (e.g. drf-yasg nesting a property under "schema").
func sanitizeDoc(doc *openapi3.T) {
	if doc == nil {
		return
	}
	pruneMalformedPaths(doc)
	ensurePathParameters(doc)
	uniquifyOperationIDs(doc)
	if doc.Components != nil {
		for _, ref := range doc.Components.Schemas {
			sanitizeSchemaRef(ref)
		}
		for _, ref := range doc.Components.Parameters {
			sanitizeParameterRef(ref)
		}
		for _, ref := range doc.Components.RequestBodies {
			sanitizeRequestBodyRef(ref)
		}
		for _, ref := range doc.Components.Responses {
			sanitizeResponseRef(ref)
		}
	}
	if doc.Paths == nil {
		return
	}
	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, p := range item.Parameters {
			sanitizeParameterRef(p)
		}
		for _, op := range item.Operations() {
			sanitizeOperation(op)
		}
	}
}

func sanitizeOperation(op *openapi3.Operation) {
	if op == nil {
		return
	}
	for _, p := range op.Parameters {
		sanitizeParameterRef(p)
	}
	if op.RequestBody != nil {
		sanitizeRequestBodyRef(op.RequestBody)
	}
	if op.Responses != nil {
		for _, r := range op.Responses.Map() {
			sanitizeResponseRef(r)
		}
	}
}

func sanitizeParameterRef(ref *openapi3.ParameterRef) {
	if ref == nil || ref.Value == nil {
		return
	}
	sanitizeSchemaRef(ref.Value.Schema)
	for _, m := range ref.Value.Content {
		if m != nil {
			sanitizeSchemaRef(m.Schema)
		}
	}
}

func sanitizeRequestBodyRef(ref *openapi3.RequestBodyRef) {
	if ref == nil || ref.Value == nil {
		return
	}
	for _, m := range ref.Value.Content {
		if m != nil {
			sanitizeSchemaRef(m.Schema)
		}
	}
}

func sanitizeResponseRef(ref *openapi3.ResponseRef) {
	if ref == nil || ref.Value == nil {
		return
	}
	for _, m := range ref.Value.Content {
		if m != nil {
			sanitizeSchemaRef(m.Schema)
		}
	}
}

func sanitizeSchemaRef(ref *openapi3.SchemaRef) {
	if ref == nil || ref.Value == nil {
		return
	}
	sanitizeSchema(ref.Value)
}

func sanitizeSchema(s *openapi3.Schema) {
	if s == nil {
		return
	}
	unwrapSchemaExtension(s)
	for _, p := range s.Properties {
		sanitizeSchemaRef(p)
	}
	if s.Items != nil {
		sanitizeSchemaRef(s.Items)
	}
	if s.Not != nil {
		sanitizeSchemaRef(s.Not)
	}
	for _, r := range s.OneOf {
		sanitizeSchemaRef(r)
	}
	for _, r := range s.AnyOf {
		sanitizeSchemaRef(r)
	}
	for _, r := range s.AllOf {
		sanitizeSchemaRef(r)
	}
	if s.AdditionalProperties.Schema != nil {
		sanitizeSchemaRef(s.AdditionalProperties.Schema)
	}
}

// unwrapSchemaExtension merges Extensions["schema"] into the Schema.
// Some generators (notably drf-yasg examples) emit property objects like
// `{ "description": "...", "schema": { "type": "object", "properties": ... } }`
// which become Schema.Extensions["schema"] after conversion and fail Validate
// with "extra sibling fields: [schema]".
func unwrapSchemaExtension(s *openapi3.Schema) {
	raw, ok := s.Extensions["schema"]
	if !ok {
		return
	}
	delete(s.Extensions, "schema")
	if len(s.Extensions) == 0 {
		s.Extensions = nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return
	}
	var nested openapi3.Schema
	if err := json.Unmarshal(b, &nested); err != nil {
		return
	}
	mergeSchemaFrom(&nested, s)
}

// pruneMalformedPaths drops path keys that are not usable OpenAPI templates.
// drf-yasg occasionally emits concatenated Django regex routes containing
// groups like ([0-9]) mixed into the path key.
func pruneMalformedPaths(doc *openapi3.T) {
	if doc == nil || doc.Paths == nil {
		return
	}
	for path := range doc.Paths.Map() {
		if isMalformedPathKey(path) {
			doc.Paths.Delete(path)
		}
	}
}

func isMalformedPathKey(path string) bool {
	if path == "" || path[0] != '/' {
		return true
	}
	if strings.ContainsAny(path, "()[]") {
		return true
	}
	// Unusually long keys usually mean multiple routes were glued together.
	if len(path) > 180 {
		return true
	}
	return false
}

// ensurePathParameters adds missing path parameters declared by {name} templates.
func ensurePathParameters(doc *openapi3.T) {
	if doc == nil || doc.Paths == nil {
		return
	}
	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		vars := pathTemplateNames(path)
		if len(vars) == 0 {
			continue
		}
		common := pathParamNames(item.Parameters)
		for _, op := range item.Operations() {
			if op == nil {
				continue
			}
			have := pathParamNames(op.Parameters)
			for k := range common {
				have[k] = true
			}
			for _, name := range vars {
				if have[name] {
					continue
				}
				op.Parameters = append(op.Parameters, &openapi3.ParameterRef{
					Value: &openapi3.Parameter{
						Name:     name,
						In:       openapi3.ParameterInPath,
						Required: true,
						Schema:   &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
					},
				})
				have[name] = true
			}
		}
	}
}

func pathTemplateNames(path string) []string {
	matches := rePathTemplate.FindAllStringSubmatch(path, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, m := range matches {
		name := m[1]
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func uniquifyOperationIDs(doc *openapi3.T) {
	if doc == nil || doc.Paths == nil {
		return
	}
	used := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for method, op := range item.Operations() {
			if op == nil {
				continue
			}
			base := strings.TrimSpace(op.OperationID)
			if base == "" {
				base = strings.ToLower(method) + "_" + slugOperationPath(path)
				op.OperationID = base
			}
			id := base
			for n := 2; used[id]; n++ {
				id = fmt.Sprintf("%s_%d", base, n)
			}
			op.OperationID = id
			used[id] = true
		}
	}
}

func slugOperationPath(path string) string {
	s := strings.Trim(path, "/")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "{", "")
	s = strings.ReplaceAll(s, "}", "")
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			return r
		default:
			return '_'
		}
	}, s)
	if s == "" {
		return "root"
	}
	return s
}

func pathParamNames(params openapi3.Parameters) map[string]bool {
	out := map[string]bool{}
	for _, p := range params {
		if p == nil || p.Value == nil {
			continue
		}
		if p.Value.In == openapi3.ParameterInPath {
			out[p.Value.Name] = true
		}
	}
	return out
}

func mergeSchemaFrom(src, dst *openapi3.Schema) {
	if src == nil || dst == nil {
		return
	}
	if len(dst.Properties) == 0 && len(src.Properties) > 0 {
		dst.Properties = src.Properties
	} else if len(src.Properties) > 0 {
		if dst.Properties == nil {
			dst.Properties = openapi3.Schemas{}
		}
		for k, v := range src.Properties {
			if _, exists := dst.Properties[k]; !exists {
				dst.Properties[k] = v
			}
		}
	}
	if dst.Items == nil && src.Items != nil {
		dst.Items = src.Items
	}
	if (dst.Type == nil || len(*dst.Type) == 0) && src.Type != nil {
		dst.Type = src.Type
	}
	if dst.Description == "" && src.Description != "" {
		dst.Description = src.Description
	}
	if len(dst.Required) == 0 && len(src.Required) > 0 {
		dst.Required = src.Required
	}
	if dst.Example == nil && src.Example != nil {
		dst.Example = src.Example
	}
	if len(dst.Enum) == 0 && len(src.Enum) > 0 {
		dst.Enum = src.Enum
	}
	if len(dst.AllOf) == 0 && len(src.AllOf) > 0 {
		dst.AllOf = src.AllOf
	}
	if len(dst.OneOf) == 0 && len(src.OneOf) > 0 {
		dst.OneOf = src.OneOf
	}
	if len(dst.AnyOf) == 0 && len(src.AnyOf) > 0 {
		dst.AnyOf = src.AnyOf
	}
}
