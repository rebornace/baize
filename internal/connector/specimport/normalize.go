package specimport

import (
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

	if baseURL != "" {
		patchServers(doc, baseURL)
	}

	loader := openapi3.NewLoader()
	if err := doc.Validate(loader.Context); err != nil {
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
	var doc2 openapi2.T
	if err := unmarshalSpec(content, &doc2); err != nil {
		return nil, fmt.Errorf("parse swagger2: %w", err)
	}
	doc3, err := openapi2conv.ToV3(&doc2)
	if err != nil {
		return nil, fmt.Errorf("convert swagger2: %w", err)
	}
	return doc3, nil
}

func unmarshalSpec(content []byte, dst any) error {
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return errEmptyContent
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		if err := json.Unmarshal(content, dst); err != nil {
			return err
		}
		return nil
	}
	if err := yaml.Unmarshal(content, dst); err != nil {
		return err
	}
	return nil
}

func patchServers(doc *openapi3.T, baseURL string) {
	doc.Servers = openapi3.Servers{
		{URL: baseURL},
	}
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
