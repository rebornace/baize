package toolindex

import (
	"regexp"
	"strings"
)

// Lexicon notes (intentionally small):
//
// Retrieval must NOT depend on ZH↔EN verb synonym tables. Cross-lingual recall
// comes from a dense Embedder (see embedder.go). What remains here is:
//   - optional filler stopwords for *routing IDF* (not translation);
//   - HTTP operationId destroy tails (Latin, from OpenAPI shapes);
//   - substring intent floors used only by preserve* helpers as a last resort.
//
// Prefer improving tool descriptions + embedding model over growing this file.

// queryStop: fillers that dilute discriminative content in deterministic
// system-routing scorers. Not a translation dictionary.
var queryStop = map[string]bool{
	// ZH fillers (incl. generic「查询」which appears in nearly every tool desc)
	"一下": true, "查询": true, "帮我": true, "最近": true, "怎么": true,
	"一个": true, "某个": true, "多少": true, "操作": true,
	// EN fillers
	"please": true, "help": true, "me": true, "my": true, "the": true,
	"a": true, "an": true, "just": true, "can": true, "could": true,
	"would": true, "want": true, "need": true, "some": true, "any": true,
	"this": true, "that": true, "for": true, "with": true, "from": true,
	"controller": true, "api": true, "list": true, "get": true, "query": true,
	"info": true, "page": true,
}

// IsStopword reports filler tokens for routing scorers.
func IsStopword(t string) bool { return queryStop[strings.ToLower(t)] }

// DestroyOpTokens are trailing operationId segments that mean "destroy state"
// on mutating HTTP methods (POST …/remove). Latin-only: operationIds are EN.
var DestroyOpTokens = map[string]bool{
	"delete": true, "remove": true, "destroy": true, "erase": true,
}

// IsDestroyOpToken reports a Latin operationId tail that destroys state.
func IsDestroyOpToken(op string) bool {
	return DestroyOpTokens[strings.ToLower(strings.TrimSpace(op))]
}

// Intent floors for preserveHTTPIntentTools (last-resort, not the retriever).
var (
	deleteIntentSubstrings = []string{
		"删除", "删掉", "删了", "删", "移除", "去掉", "注销",
		"delete", "remove", "destroy", "erase",
	}
	listIntentSubstrings = []string{
		"列出", "列表", "分页", "前几", "多少个", "几条",
		"page", "list", "show me", "show all", "show ", "how many",
		"top ",
	}
	detailIntentSubstrings = []string{
		"详情", "详细", "具体信息",
		"detail", "details", "look up", "lookup",
	}
)

// IntentVerbTokens are skipped when measuring entity overlap (verb ≠ entity).
var IntentVerbTokens = map[string]bool{
	"delete": true, "remove": true, "destroy": true, "erase": true,
	"create": true, "add": true, "insert": true, "update": true, "patch": true,
	"list": true, "page": true, "query": true, "get": true, "find": true,
	"detail": true, "details": true,
	"删除": true, "删掉": true, "移除": true, "创建": true, "新建": true,
	"修改": true, "更新": true, "列出": true, "列表": true, "分页": true,
	"详情": true, "详细": true,
}

var (
	reListTopNZH = regexp.MustCompile(`前\s*\d+\s*(个|条|名)`)
	reListPageZH = regexp.MustCompile(`第\s*\d+\s*页`)
	reListTopNEN = regexp.MustCompile(`\b(top|first|last)\s*\d+\b`)
	reListPageEN = regexp.MustCompile(`\bpage\s*\d+\b`)
	reDetailID   = regexp.MustCompile(`\bid\b\s*=?\s*\d+`)
)

// HasDestructiveIntent reports delete/destroy phrasing for preserve floors.
func HasDestructiveIntent(query string) bool {
	return hasAnySubstring(query, deleteIntentSubstrings)
}

// HasListIntent reports list/page phrasing for preserve floors.
func HasListIntent(query string) bool {
	q := strings.ToLower(query)
	if hasAnySubstring(q, listIntentSubstrings) {
		return true
	}
	return reListTopNZH.MatchString(q) || reListPageZH.MatchString(q) ||
		reListTopNEN.MatchString(q) || reListPageEN.MatchString(q)
}

// HasDetailIntent reports detail/lookup phrasing for preserve floors.
func HasDetailIntent(query string) bool {
	q := strings.ToLower(query)
	if hasAnySubstring(q, detailIntentSubstrings) {
		return true
	}
	return reDetailID.MatchString(q)
}

// IsIntentVerbToken reports a token that is a CRUD/list verb, not an entity.
func IsIntentVerbToken(t string) bool {
	return IntentVerbTokens[strings.ToLower(t)] || IntentVerbTokens[t]
}

func hasAnySubstring(query string, patterns []string) bool {
	q := strings.ToLower(query)
	for _, p := range patterns {
		if p != "" && strings.Contains(q, strings.ToLower(p)) {
			return true
		}
	}
	return false
}
