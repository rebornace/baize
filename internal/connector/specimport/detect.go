package specimport

import (
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	FormatAuto     = "auto"
	FormatOpenAPI3 = "openapi3"
	FormatSwagger2 = "swagger2"
	FormatPostman  = "postman"
)

// DetectFormat inspects raw spec bytes and returns openapi3, swagger2, postman, or "".
func DetectFormat(content []byte) string {
	root, err := parseRoot(content)
	if err != nil {
		return ""
	}

	if v, ok := root["swagger"]; ok {
		if s, ok := v.(string); ok && strings.HasPrefix(s, "2") {
			return FormatSwagger2
		}
	}

	if v, ok := root["openapi"]; ok {
		if s, ok := v.(string); ok && strings.HasPrefix(s, "3") {
			return FormatOpenAPI3
		}
	}

	if info, ok := root["info"].(map[string]any); ok {
		if schema, ok := info["schema"].(string); ok && strings.Contains(strings.ToLower(schema), "postman") {
			return FormatPostman
		}
	}

	return ""
}

func parseRoot(content []byte) (map[string]any, error) {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return nil, errEmptyContent
	}

	var root map[string]any
	if trimmed[0] == '{' || trimmed[0] == '[' {
		if err := json.Unmarshal(content, &root); err != nil {
			return nil, err
		}
		return root, nil
	}

	if err := yaml.Unmarshal(content, &root); err != nil {
		return nil, err
	}
	return root, nil
}
