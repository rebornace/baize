package run

import (
	"regexp"
	"strings"
	"unicode"
)

// memoryNearDupThreshold is the character-bigram Jaccard cutoff chosen by
// scripts/memory-dedupe-eval (zero false positives on the curated set, with
// margin above the highest distinct pair).
const memoryNearDupThreshold = 0.72

var memoryNumberPattern = regexp.MustCompile(`\d+(?:\.\d+)?`)

// nearDupText holds the precomputed features for one memory text.
type nearDupText struct {
	length int
	grams  map[string]bool
	nums   map[string]bool
}

func normalizeNearDup(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func bigramSet(norm string) map[string]bool {
	runes := []rune(norm)
	out := make(map[string]bool, len(runes))
	if len(runes) == 1 {
		out[string(runes)] = true
		return out
	}
	for i := 0; i+1 < len(runes); i++ {
		out[string(runes[i:i+2])] = true
	}
	return out
}

func numberSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, n := range memoryNumberPattern.FindAllString(s, -1) {
		out[n] = true
	}
	return out
}

func newNearDupText(s string) nearDupText {
	norm := normalizeNearDup(s)
	return nearDupText{
		length: len([]rune(norm)),
		grams:  bigramSet(norm),
		nums:   numberSet(s),
	}
}

func sameStringSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// similarTo applies the deterministic guards and bigram Jaccard.
func (t nearDupText) similarTo(other nearDupText) bool {
	if t.length == 0 || other.length == 0 {
		return false
	}
	maxLen, minLen := t.length, other.length
	if maxLen < minLen {
		maxLen, minLen = minLen, maxLen
	}
	if float64(maxLen)/float64(minLen) > 2.5 {
		return false
	}
	if len(t.nums) > 0 && len(other.nums) > 0 && !sameStringSet(t.nums, other.nums) {
		return false
	}
	inter := 0
	for g := range t.grams {
		if other.grams[g] {
			inter++
		}
	}
	union := len(t.grams) + len(other.grams) - inter
	if union == 0 {
		return false
	}
	return float64(inter)/float64(union) >= memoryNearDupThreshold
}

func nearDuplicateAny(candidate nearDupText, existing []nearDupText) bool {
	for _, e := range existing {
		if candidate.similarTo(e) {
			return true
		}
	}
	return false
}
