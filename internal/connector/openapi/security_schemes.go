package openapi

import (
	"fmt"
	"os"

	"github.com/getkin/kin-openapi/openapi3"
)

// LoadSecuritySchemes reads components.securitySchemes from an OpenAPI 3 spec file.
func LoadSecuritySchemes(specPath string) (map[string]any, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("read openapi spec: %w", err)
	}
	return LoadSecuritySchemesFromBytes(data)
}

// LoadSecuritySchemesFromBytes parses securitySchemes from OpenAPI 3 spec bytes.
func LoadSecuritySchemesFromBytes(data []byte) (map[string]any, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("load openapi: %w", err)
	}
	return securitySchemesFromDoc(doc), nil
}

func securitySchemesFromDoc(doc *openapi3.T) map[string]any {
	out := map[string]any{}
	if doc == nil || doc.Components == nil || doc.Components.SecuritySchemes == nil {
		return out
	}
	for name, ref := range doc.Components.SecuritySchemes {
		if ref == nil || ref.Value == nil {
			continue
		}
		out[name] = securitySchemeToMap(ref.Value)
	}
	return out
}

func securitySchemeToMap(s *openapi3.SecurityScheme) map[string]any {
	m := map[string]any{"type": s.Type}
	if s.Scheme != "" {
		m["scheme"] = s.Scheme
	}
	if s.In != "" {
		m["in"] = string(s.In)
	}
	if s.Name != "" {
		m["name"] = s.Name
	}
	return m
}
