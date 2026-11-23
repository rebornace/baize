// Package expect defines state-field expectations for evidence eval.
//
// A field is addressed by a dot path (array segments are numeric indexes) and
// matched with one of three operations. The default operation is exact
// equality. JSON/YAML accept either a bare scalar (exact equality) or an
// object such as {"op":"contains","value":"bug"}.
package expect

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Supported match operations.
const (
	OpEq       = "eq"
	OpContains = "contains"
	OpRegex    = "regex"
)

const (
	maxPathLen      = 256
	maxPathSegments = 16
)

var errUnknownField = errors.New("unknown field")

// Field is one expected value for a state record field.
type Field struct {
	Op    string `json:"op,omitempty" yaml:"op,omitempty"`
	Value any    `json:"value,omitempty" yaml:"value,omitempty"`
}

// EffectiveOp returns the normalized operation name.
func (f Field) EffectiveOp() string {
	if strings.TrimSpace(f.Op) == "" {
		return OpEq
	}
	return strings.TrimSpace(f.Op)
}

// Validate rejects unknown operations, empty values, and invalid regexes.
func (f Field) Validate() error {
	op := f.EffectiveOp()
	switch op {
	case OpEq:
		if f.Value == nil {
			return errors.New("value is required")
		}
		return nil
	case OpContains, OpRegex:
		s, ok := f.Value.(string)
		if !ok {
			return fmt.Errorf("%s requires a string value", op)
		}
		if op == OpRegex {
			if _, err := regexp.Compile(s); err != nil {
				return fmt.Errorf("invalid regex: %w", err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown op %q (want eq, contains, or regex)", op)
	}
}

// UnmarshalJSON accepts a bare scalar (exact equality) or an object with
// op/value. Unknown object keys are rejected.
func (f *Field) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return errors.New("empty field expectation")
	}
	if raw[0] == '{' {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		var aux struct {
			Op    string `json:"op"`
			Value any    `json:"value"`
		}
		if err := dec.Decode(&aux); err != nil {
			return err
		}
		f.Op, f.Value = aux.Op, aux.Value
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	f.Op, f.Value = OpEq, v
	return nil
}

// MarshalJSON always emits an explicit op/value object.
func (f Field) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Op    string `json:"op"`
		Value any    `json:"value"`
	}{Op: f.EffectiveOp(), Value: f.Value})
}

// UnmarshalYAML accepts a bare scalar or an object with op/value.
func (f *Field) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		var v any
		if err := node.Decode(&v); err != nil {
			return err
		}
		f.Op, f.Value = OpEq, v
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return errors.New("field expectation must be a scalar or mapping")
	}
	var op string
	var value any
	seenValue := false
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		switch key {
		case "op":
			if err := node.Content[i+1].Decode(&op); err != nil {
				return err
			}
		case "value":
			if err := node.Content[i+1].Decode(&value); err != nil {
				return err
			}
			seenValue = true
		default:
			return fmt.Errorf("%w %q", errUnknownField, key)
		}
	}
	if !seenValue {
		return errors.New("value is required")
	}
	f.Op, f.Value = op, value
	return nil
}

// MarshalYAML emits an explicit op/value mapping.
func (f Field) MarshalYAML() (any, error) {
	return map[string]any{"op": f.EffectiveOp(), "value": f.Value}, nil
}

// ValidateFields checks paths and each field's operation.
func ValidateFields(fields map[string]Field) error {
	if len(fields) == 0 {
		return errors.New("fields must not be empty")
	}
	for path, field := range fields {
		if err := ValidatePath(path); err != nil {
			return fmt.Errorf("field %q: %w", path, err)
		}
		if err := field.Validate(); err != nil {
			return fmt.Errorf("field %q: %w", path, err)
		}
	}
	return nil
}

// ValidatePath checks a dot path and its segment count.
func ValidatePath(path string) error {
	if path == "" {
		return errors.New("path is empty")
	}
	if len(path) > maxPathLen {
		return fmt.Errorf("path exceeds %d bytes", maxPathLen)
	}
	parts := strings.Split(path, ".")
	if len(parts) > maxPathSegments {
		return fmt.Errorf("path exceeds %d segments", maxPathSegments)
	}
	for _, p := range parts {
		if p == "" {
			return errors.New("path has an empty segment")
		}
	}
	return nil
}

// Resolve reads a dot path from a record. Missing maps, non-numeric array
// segments, and out-of-range indexes return false.
func Resolve(record map[string]any, path string) (any, bool) {
	var cur any = record
	for _, part := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[part]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// Match reports whether got satisfies f.
func Match(got any, f Field) bool {
	switch f.EffectiveOp() {
	case OpEq:
		return ValueString(got) == ValueString(f.Value)
	case OpContains:
		s, ok := f.Value.(string)
		return ok && strings.Contains(ValueString(got), s)
	case OpRegex:
		s, ok := f.Value.(string)
		if !ok {
			return false
		}
		re, err := regexp.Compile(s)
		return err == nil && re.MatchString(ValueString(got))
	default:
		return false
	}
}

// MatchAll reports whether every expected field matches the record. An empty
// expectation is invalid and never matches.
func MatchAll(record map[string]any, expected map[string]Field) bool {
	if len(expected) == 0 {
		return false
	}
	for path, field := range expected {
		got, ok := Resolve(record, path)
		if !ok || !Match(got, field) {
			return false
		}
	}
	return true
}

// ValueString renders a scalar for comparison; composite values fall back to
// compact JSON.
func ValueString(v any) string {
	switch n := v.(type) {
	case nil:
		return ""
	case string:
		return n
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(n), 'f', -1, 32)
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case bool:
		return strconv.FormatBool(n)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
