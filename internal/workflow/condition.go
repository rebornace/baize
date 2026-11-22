package workflow

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// Condition is a safe, declarative workflow guard. Exactly one of the
// following modes is used:
//
//	leaf:   {path, op, value?}
//	group:  {all: [...]}, {any: [...]}, or {not: {...}}
//
// Ops: eq, ne, contains, in, exists, not_exists, gt, gte, lt, lte.
type Condition struct {
	All []Condition `yaml:"all,omitempty"`
	Any []Condition `yaml:"any,omitempty"`
	Not *Condition  `yaml:"not,omitempty"`

	Path  string `yaml:"path,omitempty"`
	Op    string `yaml:"op,omitempty"`
	Value any    `yaml:"value,omitempty"`
}

var conditionPathRe = regexp.MustCompile(`^[a-zA-Z0-9_.]+$`)

var conditionOps = map[string]bool{
	"eq": true, "ne": true, "contains": true, "in": true,
	"exists": true, "not_exists": true,
	"gt": true, "gte": true, "lt": true, "lte": true,
}

func (c *Condition) Validate() error {
	if c == nil {
		return nil
	}
	modes := 0
	if len(c.All) > 0 {
		modes++
	}
	if len(c.Any) > 0 {
		modes++
	}
	if c.Not != nil {
		modes++
	}
	if c.Path != "" || c.Op != "" || c.Value != nil {
		modes++
	}
	if modes != 1 {
		return fmt.Errorf("condition: use exactly one of path/op, all, any or not")
	}
	if len(c.All) > 0 {
		for i := range c.All {
			if err := c.All[i].Validate(); err != nil {
				return err
			}
		}
		return nil
	}
	if len(c.Any) > 0 {
		for i := range c.Any {
			if err := c.Any[i].Validate(); err != nil {
				return err
			}
		}
		return nil
	}
	if c.Not != nil {
		return c.Not.Validate()
	}
	if !conditionPathRe.MatchString(c.Path) {
		return fmt.Errorf("condition: invalid path %q", c.Path)
	}
	op := strings.ToLower(strings.TrimSpace(c.Op))
	if !conditionOps[op] {
		return fmt.Errorf("condition: unsupported op %q", c.Op)
	}
	if op == "exists" || op == "not_exists" {
		return nil
	}
	if c.Value == nil {
		return fmt.Errorf("condition: op %q requires value", op)
	}
	if op == "in" {
		if _, ok := c.Value.([]any); !ok {
			return fmt.Errorf("condition: op \"in\" requires a list value")
		}
	}
	return nil
}

// EvalCondition evaluates c against the workflow tree.
func EvalCondition(c *Condition, tree map[string]any) (bool, error) {
	if c == nil {
		return true, nil
	}
	if len(c.All) > 0 {
		for i := range c.All {
			ok, err := EvalCondition(&c.All[i], tree)
			if err != nil || !ok {
				return ok, err
			}
		}
		return true, nil
	}
	if len(c.Any) > 0 {
		for i := range c.Any {
			ok, err := EvalCondition(&c.Any[i], tree)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	if c.Not != nil {
		ok, err := EvalCondition(c.Not, tree)
		return !ok, err
	}

	value, found := Resolve(tree, c.Path)
	op := strings.ToLower(strings.TrimSpace(c.Op))
	switch op {
	case "exists":
		return found && value != nil, nil
	case "not_exists":
		return !found || value == nil, nil
	case "eq":
		return found && valuesEqual(value, c.Value), nil
	case "ne":
		return !found || !valuesEqual(value, c.Value), nil
	case "contains":
		return found && containsValue(value, c.Value), nil
	case "in":
		list, ok := c.Value.([]any)
		if !ok {
			return false, fmt.Errorf("condition: op \"in\" requires a list value")
		}
		return found && listContains(list, value), nil
	case "gt", "gte", "lt", "lte":
		if !found {
			return false, nil
		}
		cmp, ok := compareValues(value, c.Value)
		if !ok {
			return false, nil
		}
		switch op {
		case "gt":
			return cmp > 0, nil
		case "gte":
			return cmp >= 0, nil
		case "lt":
			return cmp < 0, nil
		default:
			return cmp <= 0, nil
		}
	}
	return false, fmt.Errorf("condition: unsupported op %q", c.Op)
}

func valuesEqual(a, b any) bool {
	if af, ok := toFloat(a); ok {
		if bf, ok := toFloat(b); ok {
			return af == bf
		}
	}
	return reflect.DeepEqual(a, b)
}

func listContains(list []any, v any) bool {
	for _, item := range list {
		if valuesEqual(item, v) {
			return true
		}
	}
	return false
}

func containsValue(hay any, needle any) bool {
	switch h := hay.(type) {
	case string:
		s, ok := needle.(string)
		return ok && strings.Contains(h, s)
	case []any:
		return listContains(h, needle)
	default:
		return false
	}
}

func compareValues(a, b any) (int, bool) {
	if af, ok := toFloat(a); ok {
		if bf, ok := toFloat(b); ok {
			switch {
			case af < bf:
				return -1, true
			case af > bf:
				return 1, true
			default:
				return 0, true
			}
		}
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		return strings.Compare(as, bs), true
	}
	return 0, false
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err == nil && !math.IsNaN(f) {
			return f, true
		}
	}
	return 0, false
}
