package specimport_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/connector/specimport"
)

func fixturePath(name string) string {
	return filepath.Join("..", "..", "..", "examples", "spec-import", name)
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(fixturePath(name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestDetectFormatFixtures(t *testing.T) {
	tests := []struct {
		file   string
		format string
	}{
		{"openapi3-min.json", specimport.FormatOpenAPI3},
		{"swagger2-min.json", specimport.FormatSwagger2},
		{"postman-min.json", specimport.FormatPostman},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			got := specimport.DetectFormat(readFixture(t, tc.file))
			if got != tc.format {
				t.Fatalf("DetectFormat(%s) = %q, want %q", tc.file, got, tc.format)
			}
		})
	}
}

func TestNormalizeOpenAPI3Fixture(t *testing.T) {
	content := readFixture(t, "openapi3-min.json")
	out, detected, err := specimport.Normalize(content, specimport.FormatAuto, "https://api.example.com")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if detected != specimport.FormatOpenAPI3 {
		t.Fatalf("detected = %q, want openapi3", detected)
	}
	assertNormalizedHasPath(t, out, "/items")
	assertServersURL(t, out, "https://api.example.com")
}

func TestNormalizeSwagger2Fixture(t *testing.T) {
	content := readFixture(t, "swagger2-min.json")
	out, detected, err := specimport.Normalize(content, specimport.FormatAuto, "https://api.example.com")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if detected != specimport.FormatSwagger2 {
		t.Fatalf("detected = %q, want swagger2", detected)
	}
	assertNormalizedHasPath(t, out, "/items")
	assertServersURL(t, out, "https://api.example.com")
}

func TestNormalizeSwagger2NestedPropertySchema(t *testing.T) {
	// drf-yasg sometimes nests property shape under "schema", which becomes
	// Schema.Extensions["schema"] and fails Validate with extra sibling fields.
	content := []byte(`{
  "swagger": "2.0",
  "info": {"title": "t", "version": "1"},
  "paths": {
    "/q/": {
      "get": {
        "operationId": "list_q",
        "responses": {
          "200": {
            "description": "ok",
            "schema": {
              "type": "object",
              "properties": {
                "stat_display": {
                  "description": "cfg",
                  "schema": {
                    "type": "object",
                    "properties": {
                      "chart_style": {"type": "integer"}
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`)
	out, detected, err := specimport.Normalize(content, specimport.FormatAuto, "https://api.example.com")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if detected != specimport.FormatSwagger2 {
		t.Fatalf("detected = %q, want swagger2", detected)
	}
	assertNormalizedHasPath(t, out, "/q/")
}

func TestNormalizeSwagger2YAMLWithScalarTypes(t *testing.T) {
	// drf-yasg often serves Swagger 2 as YAML with `type: integer` scalars;
	// direct yaml→openapi2.T fails on kin-openapi Types.
	content := readFixture(t, "swagger2-types.yaml")
	out, detected, err := specimport.Normalize(content, specimport.FormatAuto, "https://api.example.com")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if detected != specimport.FormatSwagger2 {
		t.Fatalf("detected = %q, want swagger2", detected)
	}
	assertNormalizedHasPath(t, out, "/items")
}

func TestNormalizePostmanFixture(t *testing.T) {
	content := readFixture(t, "postman-min.json")
	out, detected, err := specimport.Normalize(content, specimport.FormatAuto, "https://api.example.com")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if detected != specimport.FormatPostman {
		t.Fatalf("detected = %q, want postman", detected)
	}
	assertNormalizedHasPath(t, out, "/items")
	assertServersURL(t, out, "https://api.example.com")
}

func assertNormalizedHasPath(t *testing.T, normalized []byte, path string) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(normalized, &doc); err != nil {
		t.Fatalf("unmarshal normalized: %v", err)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatalf("normalized JSON missing paths: %s", string(normalized))
	}
	if _, ok := paths[path]; !ok {
		t.Fatalf("normalized JSON missing path %q; paths=%v", path, paths)
	}
}

func assertServersURL(t *testing.T, normalized []byte, wantURL string) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(normalized, &doc); err != nil {
		t.Fatalf("unmarshal normalized: %v", err)
	}
	servers, ok := doc["servers"].([]any)
	if !ok || len(servers) == 0 {
		t.Fatalf("normalized JSON missing servers")
	}
	first, ok := servers[0].(map[string]any)
	if !ok {
		t.Fatalf("servers[0] not object")
	}
	got, _ := first["url"].(string)
	if got != wantURL {
		t.Fatalf("servers[0].url = %q, want %q", got, wantURL)
	}
}

func TestNormalizeUnknownFormat(t *testing.T) {
	_, _, err := specimport.Normalize([]byte(`{"foo":"bar"}`), specimport.FormatAuto, "")
	if err == nil || !strings.Contains(err.Error(), "invalid_spec") {
		t.Fatalf("expected invalid_spec, got %v", err)
	}
}
