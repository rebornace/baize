package specimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

var errEmptyContent = errors.New("empty content")

// Normalize converts supported import formats to validated OpenAPI 3 JSON.
func Normalize(content []byte, format string, baseURL string) ([]byte, string, error) {
	if len(strings.TrimSpace(string(content))) == 0 {
		return nil, "", fmt.Errorf("%w: empty content", ErrInvalidSpec)
	}

	detected := format
	if format == "" || format == FormatAuto {
		detected = DetectFormat(content)
		if detected == "" {
			return nil, "", fmt.Errorf("%w: unrecognized format; specify import_format or use OpenAPI/Postman export", ErrInvalidSpec)
		}
	}

	var (
		doc *openapi3.T
		err error
	)
	switch detected {
	case FormatOpenAPI3:
		doc, err = parseOpenAPI3(content)
	case FormatSwagger2:
		doc, err = convertSwagger2(content)
	case FormatPostman:
		doc, err = convertPostman(content)
	default:
		return nil, "", fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
	}
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrInvalidSpec, err)
	}

	sanitizeDoc(doc)

	if baseURL != "" {
		patchServers(doc, baseURL)
	}

	if err := validateForImport(doc); err != nil {
		return nil, "", fmt.Errorf("%w: validate: %w", ErrInvalidSpec, err)
	}
	if !hasOperations(doc) {
		return nil, "", fmt.Errorf("%w: no usable operations", ErrInvalidSpec)
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return nil, "", fmt.Errorf("%w: marshal: %w", ErrInvalidSpec, err)
	}
	return out, detected, nil
}

func parseOpenAPI3(content []byte) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(content)
	if err != nil {
		return nil, fmt.Errorf("parse openapi3: %w", err)
	}
	return doc, nil
}

func convertSwagger2(content []byte) (*openapi3.T, error) {
	// Always decode via JSON: yaml.v3 → openapi2.T mishandles Schema.Type
	// (string "integer" vs openapi3.Types) on real Swagger 2 / drf-yasg YAML.
	jsonBytes, err := specBytesAsJSON(content)
	if err != nil {
		return nil, fmt.Errorf("parse swagger2: %w", err)
	}
	var doc2 openapi2.T
	if err := json.Unmarshal(jsonBytes, &doc2); err != nil {
		return nil, fmt.Errorf("parse swagger2: %w", err)
	}
	doc3, err := openapi2conv.ToV3(&doc2)
	if err != nil {
		return nil, fmt.Errorf("convert swagger2: %w", err)
	}
	return doc3, nil
}

// specBytesAsJSON returns JSON encoding of a JSON or YAML OpenAPI/Swagger document.
func specBytesAsJSON(content []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return nil, errEmptyContent
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return []byte(trimmed), nil
	}
	var node any
	if err := yaml.Unmarshal(content, &node); err != nil {
		return nil, err
	}
	return json.Marshal(node)
}

func patchServers(doc *openapi3.T, baseURL string) {
	doc.Servers = openapi3.Servers{
		{URL: baseURL},
	}
}

func validateForImport(doc *openapi3.T) error {
	ctx := importValidationContext()
	if err := doc.Validate(ctx); err == nil {
		return nil
	}
	// Drop remaining paths that still fail (conflicting templates, odd params).
	prunePathsFailingValidate(doc, ctx)
	if err := doc.Validate(ctx); err != nil {
		return err
	}
	return nil
}

func importValidationContext() context.Context {
	return openapi3.WithValidationOptions(context.Background(),
		openapi3.DisableExamplesValidation(),
		openapi3.DisableSchemaDefaultsValidation(),
	)
}

func prunePathsFailingValidate(doc *openapi3.T, ctx context.Context) {
	if doc == nil || doc.Paths == nil {
		return
	}
	// Iteratively remove one failing path at a time based on Validate error text.
	for i := 0; i < doc.Paths.Len()+2; i++ {
		err := doc.Validate(ctx)
		if err == nil {
			return
		}
		path := extractInvalidPath(err.Error())
		if path == "" || doc.Paths.Value(path) == nil {
			return
		}
		doc.Paths.Delete(path)
	}
}

func extractInvalidPath(msg string) string {
	// kin-openapi: `invalid path /foo: ...` or `operation GET /foo must define...`
	for _, prefix := range []string{"invalid path ", "operation GET ", "operation POST ", "operation PUT ", "operation PATCH ", "operation DELETE ", "operation HEAD ", "operation OPTIONS ", "conflicting paths "} {
		if i := strings.Index(msg, prefix); i >= 0 {
			rest := msg[i+len(prefix):]
			if prefix == "conflicting paths " {
				// `conflicting paths "/a" and "/b"`
				if strings.HasPrefix(rest, "\"") {
					rest = rest[1:]
					if j := strings.Index(rest, "\""); j >= 0 {
						return rest[:j]
					}
				}
				continue
			}
			if strings.HasPrefix(prefix, "operation ") {
				// `GET /foo must...` already consumed method+space via prefix including method
				rest = strings.TrimSpace(rest)
			}
			end := len(rest)
			for _, sep := range []string{": ", " must ", "\" and "} {
				if j := strings.Index(rest, sep); j >= 0 && j < end {
					end = j
				}
			}
			p := strings.TrimSpace(rest[:end])
			p = strings.Trim(p, "\"")
			if strings.HasPrefix(p, "/") {
				return p
			}
		}
	}
	return ""
}

func hasOperations(doc *openapi3.T) bool {
	if doc == nil || doc.Paths == nil {
		return false
	}
	for _, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, op := range item.Operations() {
			if op != nil {
				return true
			}
		}
	}
	return false
}
