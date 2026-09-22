package run

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/rebornace/baize/internal/llm"
)

// nameTokenWeight boosts a keyword found in the tool name over one found only
// in the description: the operation name is the canonical, low-ambiguity
// signal (e.g. "order", "SkuStock").
const nameTokenWeight = 2.0

// queryStop removes CJK function-word bigrams and Latin generic words that
// appear in nearly every tool description. They carry no intent and, left in
// the query, a tool incidentally matching them steals score from the rare,
// discriminative content words (e.g. 订单, SKU).
var queryStop = map[string]bool{
	// CJK function / filler bigrams.
	"一下": true, "查询": true, "帮我": true, "最近": true, "怎么": true,
	"一个": true, "某个": true, "多少": true, "操作": true,
	// Latin generic operation words.
	"controller": true, "list": true, "get": true, "query": true,
	"info": true, "page": true,
}

// cleanQueryTokens returns the query's content tokens, dropping function words
// via queryStop. A single surviving content word is still intent-bearing.
func cleanQueryTokens(raw map[string]bool) map[string]bool {
	out := make(map[string]bool, len(raw))
	for t := range raw {
		if !queryStop[t] {
			out[t] = true
		}
	}
	return out
}

// prefilterTools deterministically narrows specs to the at most limit tools
// whose name/description best overlap the query's rare keywords. It scores
// IDF-weighted overlap over Latin word tokens and CJK bigrams so a cheap
// decision model only ever scans a short, high-recall list instead of hundreds
// of tools.
//
// If no tool shares a token with the query it returns (nil, true): the request
// maps to nothing in the catalog, and a downstream model should not be invited
// to hallucinate. This is shadow-only today, so returning no match never
// changes the tools the main model actually receives.
func prefilterTools(query string, specs []llm.ToolSpec, limit int) (picked []llm.ToolSpec, noMatch bool) {
	if limit <= 0 || len(specs) == 0 {
		return nil, true
	}

	queryTokens := cleanQueryTokens(tokenSet(query))
	if len(queryTokens) == 0 {
		return nil, true
	}

	ranked := rankWithin(queryTokens, specs)
	if len(ranked) == 0 {
		return nil, true
	}
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked, false
}

// prefilterToolsBySystem is the two-level version. Tools are grouped by
// ToolSpec.Source (connector id); each eligible system is ranked independently
// with its own IDF (so a word ubiquitous within one domain but rare within
// another is weighted correctly there), and slots are handed out round-robin
// across systems until totalLimit is reached. Round-robin guarantees every
// eligible system a minimum share instead of letting one system's tools flood
// a flat top-K, and a system with few matches simply yields its unused slots.
//
// The builtin group (Source="") is always eligible; other groups require
// allowed[s]=true. When allowed is nil every system is eligible.
func prefilterToolsBySystem(query string, specs []llm.ToolSpec, allowed map[string]bool, totalLimit int) (picked []llm.ToolSpec, noMatch bool) {
	if totalLimit <= 0 || len(specs) == 0 {
		return nil, true
	}
	queryTokens := cleanQueryTokens(tokenSet(query))
	if len(queryTokens) == 0 {
		return nil, true
	}

	bySystem, order := groupBySource(specs, allowed)
	// Rank each eligible system independently with its own local IDF, mapping
	// ranked local positions back to global spec indices.
	rankedBySystem := make(map[string][]int, len(order))
	for _, s := range order {
		members := bySystem[s]
		localSpecs := make([]llm.ToolSpec, len(members))
		for i, idx := range members {
			localSpecs[i] = specs[idx]
		}
		rankedBySystem[s] = rankWithinIndices(queryTokens, localSpecs, members)
	}

	// Round-robin merge across systems in deterministic order.
	out := make([]llm.ToolSpec, 0, totalLimit)
	for len(out) < totalLimit {
		progressed := false
		for _, s := range order {
			if len(rankedBySystem[s]) == 0 {
				continue
			}
			idx := rankedBySystem[s][0]
			rankedBySystem[s] = rankedBySystem[s][1:]
			out = append(out, specs[idx])
			progressed = true
			if len(out) == totalLimit {
				break
			}
		}
		if !progressed {
			break
		}
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, false
}

// groupBySource buckets spec indices by ToolSpec.Source, returning the
// per-source index lists and the deterministic (sorted) list of source ids.
// The builtin group ("") always comes first. Groups are filtered by allowed
// (nil means all connector groups pass; "" always passes).
func groupBySource(specs []llm.ToolSpec, allowed map[string]bool) (map[string][]int, []string) {
	groups := make(map[string][]int)
	for i, s := range specs {
		groups[s.Source] = append(groups[s.Source], i)
	}
	ids := make([]string, 0, len(groups))
	for s := range groups {
		if s == "" {
			continue
		}
		if allowed != nil && !allowed[s] {
			delete(groups, s)
			continue
		}
		ids = append(ids, s)
	}
	sort.Strings(ids)
	order := make([]string, 0, len(ids)+1)
	if _, ok := groups[""]; ok {
		order = append(order, "")
	}
	order = append(order, ids...)
	return groups, order
}

// rankWithin ranks specs by IDF-weighted query overlap computed within this
// group, returning matched specs in deterministic order (score desc, original
// index asc).
func rankWithin(queryTokens map[string]bool, specs []llm.ToolSpec) []llm.ToolSpec {
	idxs := rankWithinIndices(queryTokens, specs, nil)
	out := make([]llm.ToolSpec, 0, len(idxs))
	for _, i := range idxs {
		out = append(out, specs[i])
	}
	return out
}

// rankWithinIndices is the scoring core. It returns local spec indices ranked
// by overlap; memberMap (when non-nil) is unused for ranking but kept so the
// caller can pass global indices through, in which case specs are the local
// slice and the returned values are global indices.
func rankWithinIndices(queryTokens map[string]bool, specs []llm.ToolSpec, globalIdx []int) []int {
	nameSets := make([]map[string]bool, len(specs))
	descSets := make([]map[string]bool, len(specs))
	df := make(map[string]int)
	for i, s := range specs {
		nameSets[i] = tokenSet(s.Name)
		descSets[i] = tokenSet(firstLine(s.Description))
		seen := make(map[string]bool, len(nameSets[i])+len(descSets[i]))
		for t := range nameSets[i] {
			seen[t] = true
		}
		for t := range descSets[i] {
			seen[t] = true
		}
		for t := range seen {
			df[t]++
		}
	}

	n := float64(len(specs))
	scores := make([]float64, len(specs))
	for qt := range queryTokens {
		d, ok := df[qt]
		if !ok || d == 0 {
			continue
		}
		// IDF downweights ubiquitous terms ("查询", "list", "Controller") and
		// upweights rare, discriminative ones ("订单", "SkuStock").
		w := 1.0 + math.Log(n/float64(d))
		for i := range specs {
			// Take the STRONGEST token match per tool (max, not sum) so an
			// incidental overlap with many weak words cannot inflate a tool
			// past one that shares the query's rarest intent token.
			var s float64
			if nameSets[i][qt] {
				s = nameTokenWeight * w
			} else if descSets[i][qt] {
				s = w
			}
			if s > scores[i] {
				scores[i] = s
			}
		}
	}

	type scored struct {
		idx   int
		score float64
	}
	ranked := make([]scored, 0, len(specs))
	for i, sc := range scores {
		if sc > 0 {
			ranked = append(ranked, scored{i, sc})
		}
	}
	// Deterministic order: score desc, then original index asc.
	sort.SliceStable(ranked, func(a, b int) bool {
		if ranked[a].score != ranked[b].score {
			return ranked[a].score > ranked[b].score
		}
		return ranked[a].idx < ranked[b].idx
	})
	out := make([]int, 0, len(ranked))
	for _, r := range ranked {
		if globalIdx != nil {
			out = append(out, globalIdx[r.idx])
		} else {
			out = append(out, r.idx)
		}
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// tokenSet builds the matching token set for a piece of text. It combines:
//   - Latin/digit word tokens, split on separators and camelCase boundaries
//     ("OmsOrderController_list" -> oms, order, controller, list), lowercased;
//   - adjacent-pair (bigram) tokens of every contiguous Han run
//     ("查询订单" -> 查询, 询订, 订单).
//
// Bigrams are used for Han text because CJK has no inter-word spaces; IDF then
// discriminates rare pairs from function-word pairs.
func tokenSet(s string) map[string]bool {
	set := make(map[string]bool)
	var hanRun []rune
	flushHan := func() {
		if len(hanRun) == 0 {
			return
		}
		for i := 0; i+1 < len(hanRun); i++ {
			set[string(hanRun[i:i+2])] = true
		}
		hanRun = hanRun[:0]
	}

	var word []rune
	flushWord := func() {
		if len(word) == 0 {
			return
		}
		for _, part := range splitCamel(string(word)) {
			part = strings.ToLower(strings.TrimSpace(part))
			if part != "" {
				set[part] = true
			}
		}
		word = word[:0]
	}

	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			flushWord()
			hanRun = append(hanRun, r)
		case isLatinAlphaDigit(r):
			flushHan()
			word = append(word, r)
		default:
			flushHan()
			flushWord()
		}
	}
	flushHan()
	flushWord()
	return set
}

func isLatinAlphaDigit(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// splitCamel splits an alphanumeric identifier on underscores/digits and
// camelCase boundaries so "PmsSkuStockController" and "get_list" both yield
// their constituent words.
func splitCamel(word string) []string {
	chunks := strings.FieldsFunc(word, func(r rune) bool {
		return r == '_' || r == '-' || r >= '0' && r <= '9'
	})
	var out []string
	for _, c := range chunks {
		runes := []rune(c)
		start := 0
		for i := 1; i < len(runes); i++ {
			// lower -> upper boundary: get|List
			if unicode.IsLower(runes[i-1]) && unicode.IsUpper(runes[i]) {
				out = append(out, string(runes[start:i]))
				start = i
				continue
			}
			// upper -> upper -> lower boundary: HTTPServer -> HTTP|Server
			if i+1 < len(runes) && unicode.IsUpper(runes[i-1]) &&
				unicode.IsUpper(runes[i]) && unicode.IsLower(runes[i+1]) {
				out = append(out, string(runes[start:i]))
				start = i
			}
		}
		out = append(out, string(runes[start:]))
	}
	return out
}
