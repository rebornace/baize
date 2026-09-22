package decide

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/rebornace/baize/internal/llm"
)

// remoteMultiSystemPrompt forces a JSON array answer chosen strictly from the
// named candidates. response_format cannot be relied on across OpenAI-
// compatible endpoints, so the contract is enforced by the prompt and
// validated by parsing.
const remoteMultiSystemPrompt = `你是工具选择器。任务：从给定候选工具中，只选出完成“用户这一轮请求”直接需要的工具（通常 1-8 个）。
只输出一个 JSON 字符串数组，元素必须是候选工具里的确切名称，例如 ["tool_a","tool_b"]。
不要输出解释、markdown 代码块或候选之外的名称。
务必做减法：与本轮请求无关、用不到的工具不要选；宁少勿滥。即使不完全确定，也只选最可能用到的少数几个，不要返回全部。`

// RemoteMulti wraps an llm.Provider and coerces its reply into the subset of
// Question.Options the model chose. A chat error, unparseable reply, or empty
// option set becomes ErrUnavailable so the Chain can degrade. It is the
// pick-many counterpart of the binary Remote.
type RemoteMulti struct {
	provider llm.Provider
}

// NewRemoteMulti builds a pick-many remote around the given LLM. nil disables it.
func NewRemoteMulti(provider llm.Provider) *RemoteMulti {
	return &RemoteMulti{provider: provider}
}

// Enabled reports whether a backing provider is present.
func (r *RemoteMulti) Enabled() bool { return r != nil && r.provider != nil }

// Ask coerces the backing model's reply into the chosen option names.
func (r *RemoteMulti) Ask(ctx context.Context, q Question) (Answer, error) {
	if !r.Enabled() || len(q.Options) == 0 {
		return Answer{}, ErrUnavailable
	}
	user := strings.TrimSpace(q.Context)
	if user == "" {
		// No descriptive context supplied; fall back to bare option names.
		user = "候选工具：\n" + strings.Join(q.Options, "\n")
	}
	msg, err := r.provider.Chat(ctx, []llm.Message{
		{Role: llm.RoleSystem, Content: remoteMultiSystemPrompt},
		{Role: llm.RoleUser, Content: user},
	}, nil)
	if err != nil {
		return Answer{}, ErrUnavailable
	}

	var raw []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(msg.Content)), &raw); err != nil {
		return Answer{}, ErrUnavailable
	}

	allowed := make(map[string]bool, len(q.Options))
	for _, o := range q.Options {
		allowed[o] = true
	}
	seen := make(map[string]bool, len(raw))
	values := make([]string, 0, len(raw))
	for _, name := range raw {
		name = strings.TrimSpace(name)
		if !allowed[name] || seen[name] {
			continue
		}
		seen[name] = true
		values = append(values, name)
	}
	return Answer{Verdict: VerdictYes, Values: values, Source: SourceRemote}, nil
}
