package openapi

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	responseFieldBudget = 1600
	responseFieldDepth  = 6
	responseFieldProps  = 40
	responseDescRunes   = 80
)

// appendResponseOutline adds the success response's concrete fields to the
// tool description. Request schemas already become InputSchema; response
// objects (including fields reached through allOf, anyOf, oneOf, and $ref)
// otherwise never reach the model, so chat cannot tell which JSON fields to read.
func appendResponseOutline(desc string, op *openapi3.Operation) string {
	outline := responseFieldOutline(op)
	if outline == "" {
		return desc
	}
	block := "返回字段：\n" + outline
	if strings.TrimSpace(desc) == "" {
		return block
	}
	return strings.TrimRight(desc, "\n") + "\n\n" + block
}

func responseFieldOutline(op *openapi3.Operation) string {
	schema := successResponseSchema(op)
	if schema == nil {
		return ""
	}
	var b strings.Builder
	budget := responseFieldBudget
	writeResponseSchema(&b, "", schema, 0, false, map[*openapi3.Schema]struct{}{}, map[*openapi3.Schema]*openapi3.Schema{}, &budget)
	return strings.TrimSpace(b.String())
}

func successResponseSchema(op *openapi3.Operation) *openapi3.Schema {
	if op == nil || op.Responses == nil {
		return nil
	}
	var ref *openapi3.ResponseRef
	for _, code := range []string{"200", "201", "202", "203", "2XX", "default"} {
		if r := op.Responses.Value(code); r != nil && r.Value != nil && contentSchema(r.Value.Content) != nil {
			ref = r
			break
		}
	}
	if ref == nil {
		codes := make([]string, 0, op.Responses.Len())
		for code, r := range op.Responses.Map() {
			if r != nil && r.Value != nil && strings.HasPrefix(code, "2") && contentSchema(r.Value.Content) != nil {
				codes = append(codes, code)
			}
		}
		sort.Strings(codes)
		if len(codes) > 0 {
			ref = op.Responses.Value(codes[0])
		}
	}
	if ref == nil || ref.Value == nil {
		return nil
	}
	return contentSchema(ref.Value.Content)
}

func contentSchema(content openapi3.Content) *openapi3.Schema {
	if len(content) == 0 {
		return nil
	}
	if mt := content.Get("application/json"); schemaOf(mt) != nil {
		return schemaOf(mt)
	}
	keys := make([]string, 0, len(content))
	for k := range content {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var fallback *openapi3.Schema
	for _, key := range keys {
		schema := schemaOf(content[key])
		if schema == nil {
			continue
		}
		mime := strings.ToLower(strings.TrimSpace(strings.Split(key, ";")[0]))
		if strings.Contains(mime, "json") || mime == "*/*" {
			return schema
		}
		if fallback == nil {
			fallback = schema
		}
	}
	return fallback
}

func schemaOf(mt *openapi3.MediaType) *openapi3.Schema {
	if mt == nil || mt.Schema == nil {
		return nil
	}
	return mt.Schema.Value
}

func writeResponseSchema(b *strings.Builder, path string, raw *openapi3.Schema, depth int, omitLine bool, ancestors map[*openapi3.Schema]struct{}, cache map[*openapi3.Schema]*openapi3.Schema, budget *int) {
	if raw == nil || *budget <= 0 {
		return
	}
	schema := concreteSchema(raw, cache)
	if schema == nil {
		return
	}
	if _, cyc := ancestors[schema]; cyc {
		if path != "" {
			writeOutlineLine(b, path+": "+fallbackLabel(schemaLabel(schema))+"，结构同前", budget)
		}
		return
	}
	ancestors[schema] = struct{}{}
	defer delete(ancestors, schema)

	desc := firstText(raw.Description, schema.Description)
	props := collectProperties(schema, map[*openapi3.Schema]struct{}{})
	items := schemaItems(schema)
	label := schemaLabel(schema)
	if label == "" && (len(props) > 0 || items != nil) {
		label = "object"
	}
	if items != nil {
		label = "array<" + arrayItemLabel(items, cache) + ">"
	}
	if len(props) == 0 && items == nil {
		if adopted, adoptedItems, ok := adoptBranches(schema); ok {
			props = adopted
			items = adoptedItems
			if items != nil {
				label = "array<" + arrayItemLabel(items, cache) + ">"
			} else if label == "" || label == "object" {
				label = "object"
			}
		}
	}
	if label == "" {
		label = "any"
	}

	if path == "" && strings.TrimSpace(desc) != "" {
		writeOutlineLine(b, "说明："+oneLine(desc), budget)
	}

	if path != "" && !omitLine {
		line := path + ": " + label
		line += schemaNotes(schema, desc)
		if depth >= responseFieldDepth && (len(props) > 0 || items != nil) {
			line += "，更深层字段略"
			writeOutlineLine(b, line, budget)
			return
		}
		writeOutlineLine(b, line, budget)
	}

	if items != nil {
		item := concreteSchema(items, cache)
		if itemHasFields(item) {
			next := "[]"
			if path != "" {
				next = path + "[]"
			}
			writeResponseSchema(b, next, item, depth+1, true, ancestors, cache, budget)
		}
		return
	}

	if len(props) == 0 {
		if star := additionalPropertySchema(schema); star != nil {
			child := "*"
			if path != "" {
				child = path + ".*"
			}
			writeResponseSchema(b, child, star, depth+1, false, ancestors, cache, budget)
			return
		}
		if obj, ok := exampleObject(schema.Example); ok {
			if path == "" {
				writeOutlineLine(b, "（按文档示例）", budget)
			}
			writeExampleFields(b, path, obj, depth, budget)
		}
		return
	}

	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	extra := 0
	if len(names) > responseFieldProps {
		extra = len(names) - responseFieldProps
		names = names[:responseFieldProps]
	}
	for _, name := range names {
		if *budget <= 0 {
			return
		}
		child := name
		if path != "" {
			child = path + "." + name
		}
		writeResponseSchema(b, child, props[name], depth+1, false, ancestors, cache, budget)
	}
	if extra > 0 {
		writeOutlineLine(b, pathNote(path)+"该层还有 "+strconv.Itoa(extra)+" 个字段", budget)
	}
	if star := additionalPropertySchema(schema); star != nil && len(props) > 0 {
		child := "*"
		if path != "" {
			child = path + ".*"
		}
		writeResponseSchema(b, child, star, depth+1, false, ancestors, cache, budget)
	}
}

func arrayItemLabel(items *openapi3.Schema, cache map[*openapi3.Schema]*openapi3.Schema) string {
	item := concreteSchema(items, cache)
	inner := schemaLabel(item)
	if inner == "" && itemHasFields(item) {
		inner = "object"
	}
	if inner == "" {
		return "any"
	}
	return inner
}

func itemHasFields(item *openapi3.Schema) bool {
	if item == nil {
		return false
	}
	if len(collectProperties(item, map[*openapi3.Schema]struct{}{})) > 0 || schemaItems(item) != nil {
		return true
	}
	if _, ok := exampleObject(item.Example); ok {
		return true
	}
	return len(item.AllOf) > 0 || len(item.AnyOf) > 0 || len(item.OneOf) > 0
}

func fallbackLabel(label string) string {
	if label == "" {
		return "object"
	}
	return label
}

func pathNote(path string) string {
	if path == "" {
		return ""
	}
	return path + "："
}

func schemaNotes(schema *openapi3.Schema, desc string) string {
	var parts []string
	if schema.Nullable {
		parts = append(parts, "可空")
	}
	if enum := enumText(schema.Enum); enum != "" {
		parts = append(parts, "可选 "+enum)
	}
	if text := oneLine(desc); text != "" {
		parts = append(parts, text)
	}
	if len(parts) == 0 {
		return ""
	}
	return "，" + strings.Join(parts, "，")
}

func enumText(values []any) string {
	if len(values) == 0 {
		return ""
	}
	limit := len(values)
	if limit > 8 {
		limit = 8
	}
	parts := make([]string, 0, limit)
	for _, v := range values[:limit] {
		s := enumValue(v)
		if s == "" {
			continue
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return ""
	}
	out := strings.Join(parts, "、")
	if len(values) > limit {
		out += "…"
	}
	return out
}

func enumValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case json.Number:
		return t.String()
	default:
		return strings.TrimSpace(strings.ReplaceAll(jsonScalar(v), "\n", " "))
	}
}

func jsonScalar(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return strings.Trim(string(b), `"`)
}

func collectProperties(schema *openapi3.Schema, seen map[*openapi3.Schema]struct{}) map[string]*openapi3.Schema {
	out := map[string]*openapi3.Schema{}
	var walk func(*openapi3.Schema)
	walk = func(cur *openapi3.Schema) {
		if cur == nil {
			return
		}
		if _, ok := seen[cur]; ok {
			return
		}
		seen[cur] = struct{}{}
		defer delete(seen, cur)
		for name, ref := range cur.Properties {
			if ref == nil || ref.Value == nil {
				continue
			}
			if _, exists := out[name]; !exists {
				out[name] = ref.Value
			}
		}
		for _, ref := range cur.AllOf {
			if ref != nil {
				walk(ref.Value)
			}
		}
	}
	walk(schema)
	return out
}

func adoptBranches(schema *openapi3.Schema) (map[string]*openapi3.Schema, *openapi3.Schema, bool) {
	branches := compositionBranches(schema)
	if len(branches) == 0 {
		return nil, nil, false
	}
	props := map[string]*openapi3.Schema{}
	var items *openapi3.Schema
	for _, branch := range branches {
		for name, prop := range collectProperties(branch, map[*openapi3.Schema]struct{}{}) {
			if _, exists := props[name]; !exists {
				props[name] = prop
			}
		}
		if items == nil {
			items = schemaItems(branch)
		}
	}
	if len(props) == 0 && items == nil {
		return nil, nil, false
	}
	return props, items, true
}

func compositionBranches(schema *openapi3.Schema) []*openapi3.Schema {
	refs := schema.AnyOf
	if len(refs) == 0 {
		refs = schema.OneOf
	}
	var out []*openapi3.Schema
	for _, ref := range refs {
		if ref == nil || ref.Value == nil || isNullSchema(ref.Value) {
			continue
		}
		out = append(out, ref.Value)
	}
	return out
}

func schemaItems(schema *openapi3.Schema) *openapi3.Schema {
	if schema == nil || schema.Items == nil {
		return nil
	}
	return schema.Items.Value
}

func additionalPropertySchema(schema *openapi3.Schema) *openapi3.Schema {
	ref := schema.AdditionalProperties.Schema
	if ref == nil || ref.Value == nil {
		return nil
	}
	value := ref.Value
	if declaredType(value) == "" && len(value.Properties) == 0 && value.Items == nil && len(value.AllOf) == 0 {
		return nil
	}
	return value
}

func concreteSchema(schema *openapi3.Schema, cache map[*openapi3.Schema]*openapi3.Schema) *openapi3.Schema {
	if schema == nil {
		return nil
	}
	if cached, ok := cache[schema]; ok {
		return cached
	}
	nested := extensionSchema(schema)
	if nested == nil {
		cache[schema] = schema
		return schema
	}
	if len(schema.Properties) == 0 && schema.Items == nil && len(schema.AllOf) == 0 && len(schema.AnyOf) == 0 && len(schema.OneOf) == 0 {
		cache[schema] = nested
		return nested
	}
	cache[schema] = schema
	return schema
}

func extensionSchema(schema *openapi3.Schema) *openapi3.Schema {
	if schema == nil || len(schema.Extensions) == 0 {
		return nil
	}
	raw, ok := schema.Extensions["schema"]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var nested openapi3.Schema
	if err := json.Unmarshal(encoded, &nested); err != nil {
		return nil
	}
	if len(nested.Properties) == 0 && nested.Items == nil && len(nested.AllOf) == 0 && declaredType(&nested) == "" {
		return nil
	}
	return &nested
}

func schemaLabel(schema *openapi3.Schema) string {
	if schema == nil {
		return ""
	}
	if len(schema.AnyOf) > 0 || len(schema.OneOf) > 0 {
		var parts []string
		seen := map[string]bool{}
		for _, branch := range compositionBranches(schema) {
			label := declaredType(branch)
			if label == "" && (len(branch.Properties) > 0 || len(branch.AllOf) > 0) {
				label = "object"
			}
			if branch.Items != nil {
				label = "array"
			}
			if label == "" || seen[label] {
				continue
			}
			seen[label] = true
			parts = append(parts, label)
		}
		if len(parts) > 0 && declaredType(schema) == "" && len(schema.Properties) == 0 {
			return strings.Join(parts, "|")
		}
	}
	typ := declaredType(schema)
	if typ == "" {
		return ""
	}
	if schema.Format != "" && (typ == "string" || typ == "integer" || typ == "number") {
		return typ + "(" + schema.Format + ")"
	}
	return typ
}

func declaredType(schema *openapi3.Schema) string {
	if schema == nil || schema.Type == nil {
		return ""
	}
	types := schema.Type.Slice()
	if len(types) == 0 {
		return ""
	}
	return types[0]
}

func isNullSchema(schema *openapi3.Schema) bool {
	return declaredType(schema) == "null"
}

func exampleObject(value any) (map[string]any, bool) {
	obj, ok := value.(map[string]any)
	if !ok || len(obj) == 0 {
		return nil, false
	}
	return obj, true
}

func writeExampleFields(b *strings.Builder, path string, obj map[string]any, depth int, budget *int) {
	if depth >= responseFieldDepth || *budget <= 0 {
		return
	}
	names := make([]string, 0, len(obj))
	for name := range obj {
		names = append(names, name)
	}
	sort.Strings(names)
	extra := 0
	if len(names) > responseFieldProps {
		extra = len(names) - responseFieldProps
		names = names[:responseFieldProps]
	}
	for _, name := range names {
		if *budget <= 0 {
			return
		}
		child := name
		if path != "" {
			child = path + "." + name
		}
		value := obj[name]
		label := exampleLabel(value)
		writeOutlineLine(b, child+": "+label, budget)
		switch typed := value.(type) {
		case map[string]any:
			writeExampleFields(b, child, typed, depth+1, budget)
		case []any:
			if len(typed) == 0 {
				continue
			}
			if item, ok := typed[0].(map[string]any); ok {
				writeExampleFields(b, child+"[]", item, depth+1, budget)
			}
		}
	}
	if extra > 0 {
		writeOutlineLine(b, pathNote(path)+"该层还有 "+strconv.Itoa(extra)+" 个字段", budget)
	}
}

func exampleLabel(value any) string {
	switch typed := value.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		if typed == math.Trunc(typed) {
			return "integer"
		}
		return "number"
	case int, int32, int64, uint, uint32, uint64, json.Number:
		return "integer"
	case map[string]any:
		return "object"
	case []any:
		if len(typed) == 0 {
			return "array"
		}
		return "array<" + exampleLabel(typed[0]) + ">"
	default:
		return "any"
	}
}

func firstText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func oneLine(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return ""
	}
	if utf8.RuneCountInString(text) <= responseDescRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:responseDescRunes]) + "…"
}

func writeOutlineLine(b *strings.Builder, line string, budget *int) {
	if *budget <= 0 {
		return
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	cost := utf8.RuneCountInString(line) + 1
	if cost > *budget {
		note := "…（其余字段略）"
		if utf8.RuneCountInString(note)+1 <= *budget {
			b.WriteString(note)
			b.WriteByte('\n')
		}
		*budget = 0
		return
	}
	b.WriteString(line)
	b.WriteByte('\n')
	*budget -= cost
}
