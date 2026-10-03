package decide

import (
	"context"
	"regexp"
	"strings"
)

// rulesShortMaxRunes is the length below which a context is treated as short
// unless an explicit fact cue is present. Tuning lives in one place.
const rulesShortMaxRunes = 50

// factCues are Chinese/English hints that a turn carries a durable fact.
var factCues = []string{
	"记住", "别忘了", "我的工号", "我的账号", "我的账户", "密码", "地址",
	"电话", "号码", "偏好", "生日", "remember that", "my number",
}

// digitRun matches a run of >=3 digits (ids, phone fragments, numbers).
var digitRun = regexp.MustCompile(`[0-9]{3,}`)

// Rules is the zero-latency deterministic implementation. It is always
// enabled and forms the final backstop in a Chain. For a Kind it does not
// understand it abstains (ErrUnavailable) rather than guess.
type Rules struct{}

// NewRules builds the deterministic rules provider.
func NewRules() *Rules { return &Rules{} }

// Enabled always returns true.
func (r *Rules) Enabled() bool { return true }

// Ask applies the deterministic heuristic for the given Kind.
func (r *Rules) Ask(_ context.Context, q Question) (Answer, error) {
	switch q.Kind {
	case KindMemoryExtract:
		return r.memory(q)
	default:
		// Unknown decision point: defer to other implementations.
		return Answer{}, ErrUnavailable
	}
}

// memory decides whether a turn is worth a memory-extraction call. Short
// chitchat with no fact cue returns No (skip the call); an explicit cue or a
// substantial length returns Yes.
func (r *Rules) memory(q Question) (Answer, error) {
	ctxStr := q.Context
	hasCue := false
	for _, cue := range factCues {
		if strings.Contains(ctxStr, cue) {
			hasCue = true
			break
		}
	}
	if !hasCue && digitRun.MatchString(ctxStr) {
		hasCue = true
	}
	rlen := len([]rune(ctxStr))
	if !hasCue && rlen < rulesShortMaxRunes {
		return Answer{Verdict: VerdictNo, Source: SourceRules}, nil
	}
	return Answer{Verdict: VerdictYes, Source: SourceRules}, nil
}
