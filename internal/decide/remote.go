package decide

import (
	"context"
	"regexp"
	"strings"

	"github.com/rebornace/baize/internal/llm"
)

// remoteSystemPrompt forces a single-token enum answer. baize cannot rely on
// response_format (not all OpenAI-compatible endpoints expose it), so the
// contract is enforced by the prompt and validated by parsing.
const remoteSystemPrompt = `你是判断器。只回答 "yes" 或 "no"，不要输出任何其它文字、标点或解释。`

// yesWord/noWord match the enum as standalone ASCII words (case-insensitive).
var (
	yesWord = regexp.MustCompile(`(?i)\byes\b`)
	noWord  = regexp.MustCompile(`(?i)\bno\b`)
)

// Remote wraps an llm.Provider (typically a cheap/small model profile) and
// coerces its reply to an enum verdict. A chat error or an unparseable reply
// becomes ErrUnavailable so the Chain can degrade.
type Remote struct {
	provider llm.Provider
}

// NewRemote builds a remote provider around the given LLM. nil disables it.
func NewRemote(provider llm.Provider) *Remote { return &Remote{provider: provider} }

// Enabled reports whether a backing provider is present.
func (r *Remote) Enabled() bool { return r != nil && r.provider != nil }

// Ask coerces the backing model's reply to a binary verdict.
func (r *Remote) Ask(ctx context.Context, q Question) (Answer, error) {
	if !r.Enabled() {
		return Answer{}, ErrUnavailable
	}
	msg, err := r.provider.Chat(ctx, []llm.Message{
		{Role: llm.RoleSystem, Content: remoteSystemPrompt},
		{Role: llm.RoleUser, Content: q.Context},
	}, nil)
	if err != nil {
		return Answer{}, ErrUnavailable
	}
	content := strings.TrimSpace(msg.Content)
	yi := yesWord.FindStringIndex(content)
	ni := noWord.FindStringIndex(content)
	switch {
	case yi == nil && ni == nil:
		return Answer{}, ErrUnavailable
	case yi == nil:
		return Answer{Verdict: VerdictNo, Source: SourceRemote}, nil
	case ni == nil:
		return Answer{Verdict: VerdictYes, Source: SourceRemote}, nil
	default:
		// Both appear: take the first one mentioned.
		if yi[0] < ni[0] {
			return Answer{Verdict: VerdictYes, Source: SourceRemote}, nil
		}
		return Answer{Verdict: VerdictNo, Source: SourceRemote}, nil
	}
}
