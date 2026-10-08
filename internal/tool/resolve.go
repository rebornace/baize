package tool

import (
	"fmt"
	"sort"
	"strings"
)

// verbSynonyms maps a trailing operation token to interchangeable verbs.
// Used when the model invents UsersAdminController_delete but the catalog
// only has UsersAdminController_remove.
var verbSynonyms = map[string][]string{
	"delete":   {"remove", "destroy", "erase"},
	"remove":   {"delete", "destroy", "erase"},
	"destroy":  {"delete", "remove"},
	"erase":    {"delete", "remove"},
	"update":   {"patch", "edit", "modify"},
	"patch":    {"update", "edit", "modify"},
	"edit":     {"update", "patch", "modify"},
	"modify":   {"update", "patch", "edit"},
	"create":   {"add", "insert", "post"},
	"add":      {"create", "insert", "post"},
	"insert":   {"create", "add"},
	"post":     {"create", "add"},
	"get":      {"find", "fetch", "read", "detail", "findone"},
	"find":     {"get", "fetch", "read", "detail", "findone"},
	"findone":  {"get", "find", "detail"},
	"fetch":    {"get", "find", "read"},
	"read":     {"get", "find", "fetch"},
	"list":     {"page", "query", "search", "findpage"},
	"page":     {"list", "query", "findpage"},
	"findpage": {"list", "page", "query"},
	"query":    {"list", "page", "search"},
	"search":   {"list", "query"},
}

// ResolveToolName maps a model-requested tool name onto a catalog name.
// Exact matches win. Otherwise a unique high-confidence synonym rewrite of the
// trailing verb (same Controller_ prefix) is accepted so the run can continue
// into HITL / Invoke instead of failing as "unknown tool".
// Ambiguous or low-confidence misses return ok=false; use SuggestToolNames for hints.
func ResolveToolName(requested string, catalog []string) (resolved string, ok bool) {
	requested = strings.TrimSpace(requested)
	if requested == "" || len(catalog) == 0 {
		return "", false
	}
	set := make(map[string]struct{}, len(catalog))
	for _, n := range catalog {
		set[n] = struct{}{}
	}
	if _, hit := set[requested]; hit {
		return requested, true
	}

	matches := synonymPrefixMatches(requested, set)
	if len(matches) == 1 {
		return matches[0], true
	}
	if len(matches) > 1 {
		return "", false
	}

	// Unique near-miss under the same prefix (edit distance ≤ 2 on the full name).
	prefix, _, hasPrefix := splitTrailing(requested)
	if !hasPrefix {
		return "", false
	}
	var near []string
	for _, n := range catalog {
		p, _, ok := splitTrailing(n)
		if !ok || !strings.EqualFold(p, prefix) {
			continue
		}
		if levenshtein(strings.ToLower(requested), strings.ToLower(n)) <= 2 {
			near = append(near, n)
		}
	}
	if len(near) == 1 {
		return near[0], true
	}
	return "", false
}

// SuggestToolNames returns up to limit catalog names close to requested, for
// error messages when auto-resolve is not safe.
func SuggestToolNames(requested string, catalog []string, limit int) []string {
	requested = strings.TrimSpace(requested)
	if requested == "" || limit <= 0 {
		return nil
	}
	set := make(map[string]struct{}, len(catalog))
	for _, n := range catalog {
		set[n] = struct{}{}
	}
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	for _, m := range synonymPrefixMatches(requested, set) {
		add(m)
		if len(out) >= limit {
			return out
		}
	}
	type scored struct {
		name string
		dist int
	}
	var ranked []scored
	reqLower := strings.ToLower(requested)
	prefix, _, hasPrefix := splitTrailing(requested)
	for _, n := range catalog {
		if hasPrefix {
			p, _, ok := splitTrailing(n)
			if !ok || !strings.EqualFold(p, prefix) {
				continue
			}
		}
		d := levenshtein(reqLower, strings.ToLower(n))
		if d > 8 {
			continue
		}
		ranked = append(ranked, scored{n, d})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].dist != ranked[j].dist {
			return ranked[i].dist < ranked[j].dist
		}
		return ranked[i].name < ranked[j].name
	})
	for _, r := range ranked {
		add(r.name)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func synonymPrefixMatches(requested string, set map[string]struct{}) []string {
	prefix, verb, ok := splitTrailing(requested)
	if !ok {
		return nil
	}
	alts := append([]string{verb}, verbSynonyms[strings.ToLower(verb)]...)
	seen := map[string]bool{}
	var matches []string
	for _, alt := range alts {
		if alt == "" {
			continue
		}
		// Preserve catalog casing by scanning set for EqualFold match.
		candLower := strings.ToLower(prefix + "_" + alt)
		for name := range set {
			if strings.ToLower(name) == candLower && !seen[name] {
				seen[name] = true
				matches = append(matches, name)
			}
		}
		// Also accept camelCase trailing verbs: findOne vs find_one already split;
		// FindPage style without underscore after Controller is handled by splitTrailing.
	}
	sort.Strings(matches)
	return matches
}

func splitTrailing(name string) (prefix, verb string, ok bool) {
	i := strings.LastIndex(name, "_")
	if i <= 0 || i == len(name)-1 {
		return "", "", false
	}
	return name[:i], name[i+1:], true
}

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			cur[j] = del
			if ins < cur[j] {
				cur[j] = ins
			}
			if sub < cur[j] {
				cur[j] = sub
			}
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// ResolveName returns the catalog tool to use for a requested name.
// On exact or unique high-confidence synonym match it returns the catalog name.
// Otherwise it returns an error that lists near suggestions when available.
func (r *Registry) ResolveName(requested string) (string, error) {
	r.mu.RLock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	r.mu.RUnlock()
	sort.Strings(names)

	if resolved, ok := ResolveToolName(requested, names); ok {
		return resolved, nil
	}
	suggest := SuggestToolNames(requested, names, 3)
	if len(suggest) == 0 {
		return "", fmt.Errorf("unknown tool: %s", requested)
	}
	return "", fmt.Errorf("unknown tool: %s (did you mean: %s)", requested, strings.Join(suggest, ", "))
}
