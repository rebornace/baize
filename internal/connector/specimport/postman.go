package specimport

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type postmanCollection struct {
	Info struct {
		Name   string `json:"name"`
		Schema string `json:"schema"`
	} `json:"info"`
	Item []postmanItem `json:"item"`
}

type postmanItem struct {
	Name    string         `json:"name"`
	Request postmanRequest `json:"request"`
}

type postmanRequest struct {
	Method string          `json:"method"`
	URL    json.RawMessage `json:"url"`
	Body   *postmanBody    `json:"body"`
}

type postmanBody struct {
	Mode string `json:"mode"`
	Raw  string `json:"raw"`
}

type postmanURL struct {
	Raw   string              `json:"raw"`
	Path  []string            `json:"path"`
	Query []postmanQueryParam `json:"query"`
}

type postmanQueryParam struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var postmanPathParam = regexp.MustCompile(`:([A-Za-z0-9_]+)`)

func convertPostman(content []byte) (*openapi3.T, error) {
	var col postmanCollection
	if err := json.Unmarshal(content, &col); err != nil {
		return nil, fmt.Errorf("parse postman: %w", err)
	}
	if col.Info.Schema != "" && !strings.Contains(strings.ToLower(col.Info.Schema), "postman") {
		return nil, fmt.Errorf("unsupported postman schema %q", col.Info.Schema)
	}
	if len(col.Item) == 0 {
		return nil, fmt.Errorf("postman collection has no items")
	}

	title := col.Info.Name
	if title == "" {
		title = "Postman Collection"
	}

	doc := &openapi3.T{
		OpenAPI: "3.0.3",
		Info: &openapi3.Info{
			Title:   title,
			Version: "1.0.0",
		},
		Paths: openapi3.NewPaths(),
	}

	for _, item := range col.Item {
		if err := addPostmanItem(doc, item); err != nil {
			return nil, err
		}
	}
	if doc.Paths.Len() == 0 {
		return nil, fmt.Errorf("postman collection has no supported requests")
	}
	return doc, nil
}

func addPostmanItem(doc *openapi3.T, item postmanItem) error {
	method := strings.ToUpper(strings.TrimSpace(item.Request.Method))
	if method != "GET" && method != "POST" {
		return nil
	}

	path, queryParams, err := postmanPathAndQuery(item.Request.URL)
	if err != nil {
		return fmt.Errorf("request %q: %w", item.Name, err)
	}
	if path == "" {
		return fmt.Errorf("request %q: missing path", item.Name)
	}

	pathItem := doc.Paths.Find(path)
	if pathItem == nil {
		pathItem = &openapi3.PathItem{}
	}

	op := &openapi3.Operation{
		OperationID: sanitizeOperationID(item.Name),
		Responses: openapi3.NewResponses(
			openapi3.WithStatus(200, &openapi3.ResponseRef{
				Value: &openapi3.Response{Description: strPtr("ok")},
			}),
		),
	}

	for _, q := range queryParams {
		if q.Key == "" {
			continue
		}
		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name: q.Key,
				In:   openapi3.ParameterInQuery,
				Schema: &openapi3.SchemaRef{
					Value: &openapi3.Schema{Type: &openapi3.Types{"string"}},
				},
			},
		})
	}

	if method == "POST" && item.Request.Body != nil && item.Request.Body.Mode == "raw" && item.Request.Body.Raw != "" {
		schema := inferJSONBodySchema(item.Request.Body.Raw)
		op.RequestBody = &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Required: true,
				Content: openapi3.Content{
					"application/json": {
						Schema: &openapi3.SchemaRef{Value: schema},
					},
				},
			},
		}
	}

	switch method {
	case "GET":
		pathItem.Get = op
	case "POST":
		pathItem.Post = op
	}

	doc.Paths.Set(path, pathItem)
	return nil
}

func postmanPathAndQuery(raw json.RawMessage) (string, []postmanQueryParam, error) {
	if len(raw) == 0 {
		return "", nil, fmt.Errorf("missing url")
	}

	var asString string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &asString); err != nil {
			return "", nil, err
		}
		u, err := url.Parse(asString)
		if err != nil {
			return "", nil, err
		}
		path := normalizePostmanPath(u.Path)
		var query []postmanQueryParam
		for key, vals := range u.Query() {
			val := ""
			if len(vals) > 0 {
				val = vals[0]
			}
			query = append(query, postmanQueryParam{Key: key, Value: val})
		}
		return path, query, nil
	}

	var u postmanURL
	if err := json.Unmarshal(raw, &u); err != nil {
		return "", nil, err
	}
	path := normalizePostmanPath("/" + strings.Join(u.Path, "/"))
	return path, u.Query, nil
}

func normalizePostmanPath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = postmanPathParam.ReplaceAllString(path, "{$1}")
	path = strings.ReplaceAll(path, "{{baseUrl}}", "")
	path = strings.ReplaceAll(path, "{{base_url}}", "")
	if path == "" {
		return "/"
	}
	return path
}

func inferJSONBodySchema(raw string) *openapi3.Schema {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return &openapi3.Schema{Type: &openapi3.Types{"object"}}
	}
	switch typed := v.(type) {
	case map[string]any:
		props := map[string]*openapi3.SchemaRef{}
		for key, val := range typed {
			props[key] = &openapi3.SchemaRef{Value: schemaForValue(val)}
		}
		return &openapi3.Schema{
			Type:       &openapi3.Types{"object"},
			Properties: props,
		}
	default:
		return schemaForValue(typed)
	}
}

func schemaForValue(v any) *openapi3.Schema {
	switch v.(type) {
	case bool:
		return &openapi3.Schema{Type: &openapi3.Types{"boolean"}}
	case float64:
		return &openapi3.Schema{Type: &openapi3.Types{"number"}}
	case string:
		return &openapi3.Schema{Type: &openapi3.Types{"string"}}
	case []any:
		return &openapi3.Schema{Type: &openapi3.Types{"array"}}
	default:
		return &openapi3.Schema{Type: &openapi3.Types{"object"}}
	}
}

func sanitizeOperationID(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "operation"
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "operation"
	}
	return out
}

func strPtr(s string) *string { return &s }
