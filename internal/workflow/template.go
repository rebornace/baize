package workflow

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// phRe matches {{ path }} placeholders: dot-separated identifiers or numeric
// indexes. Used only to locate placeholders; surrounding text is kept.
var phRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+?)\s*\}\}`)

// Resolve walks a dot path over the data tree, descending through
// map[string]any by key and []any by numeric index. Returns (value, found).
func Resolve(tree map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var cur any = tree
	for _, p := range parts {
		if m, ok := cur.(map[string]any); ok {
			cur, ok = m[p]
			if !ok {
				return nil, false
			}
			continue
		}
		if s, ok := cur.([]any); ok {
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(s) {
				return nil, false
			}
			cur = s[idx]
			continue
		}
		return nil, false
	}
	return cur, true
}

// RenderArg renders one scalar argument value. Non-string values pass through
// unchanged. A string consisting of exactly one placeholder resolves to the
// referenced value with its native type preserved (bool stays bool); a string
// mixing placeholders and literal text concatenates all resolved values as
// "%v". Returns ok=false when a placeholder path cannot be resolved, or when
// the string contains "{{" that does not form a resolvable placeholder — no
// malformed reference is ever passed through. The execution path uses
// TryRenderArgs so unresolved references fail fast instead of rendering
// partial output.
func RenderArg(v any, tree map[string]any) (any, bool) {
	s, isStr := v.(string)
	if !isStr {
		return v, true
	}
	locs := phRe.FindAllStringSubmatchIndex(s, -1)
	if len(locs) == 1 && locs[0][0] == 0 && locs[0][1] == len(s) {
		val, found := Resolve(tree, s[locs[0][2]:locs[0][3]])
		if !found {
			return nil, false
		}
		return val, true // 整值占位：保原生类型
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		if strings.Contains(s[last:loc[0]], "{{") {
			return nil, false // 未匹配的 "{{" 混在字面文本里：fail-fast
		}
		val, found := Resolve(tree, s[loc[2]:loc[3]])
		if !found {
			return nil, false
		}
		b.WriteString(s[last:loc[0]])
		fmt.Fprintf(&b, "%v", val)
		last = loc[1]
	}
	tail := s[last:]
	if strings.Contains(tail, "{{") {
		return nil, false
	}
	if last == 0 {
		return s, true // 无占位符且无残留 "{{"
	}
	b.WriteString(tail)
	return b.String(), true
}

// RenderArgs deep-renders every scalar inside args against tree, recursing into
// maps and slices; it panics on an unresolvable reference. Kept for tests only
// — the execution path must use TryRenderArgs.
func RenderArgs(args map[string]any, tree map[string]any) map[string]any {
	out := renderAny(args, tree).(map[string]any)
	return out
}

func renderAny(v any, tree map[string]any) any {
	switch n := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(n))
		for k, vv := range n {
			m[k] = renderAny(vv, tree)
		}
		return m
	case []any:
		s := make([]any, len(n))
		for i, vv := range n {
			s[i] = renderAny(vv, tree)
		}
		return s
	default:
		r, ok := RenderArg(n, tree)
		if !ok {
			panic(fmt.Sprintf("template reference not found: %v", n))
		}
		return r
	}
}

// TryRenderArgs deep-renders args like RenderArgs but fails soft-and-fast: on
// the first unresolvable reference it records an error naming that value,
// returns it (nil at that position in the map), and keeps walking only to skip
// the remaining work without panicking.
func TryRenderArgs(args map[string]any, tree map[string]any) (map[string]any, error) {
	var bad error
	var walk func(any) any
	walk = func(v any) any {
		switch n := v.(type) {
		case map[string]any:
			m := make(map[string]any, len(n))
			for k, vv := range n {
				m[k] = walk(vv)
			}
			return m
		case []any:
			s := make([]any, len(n))
			for i, vv := range n {
				s[i] = walk(vv)
			}
			return s
		default:
			r, ok := RenderArg(n, tree)
			if !ok {
				if bad == nil {
					bad = fmt.Errorf("template reference not found: %v", n)
				}
				return nil
			}
			return r
		}
	}
	out := walk(args).(map[string]any)
	return out, bad
}
