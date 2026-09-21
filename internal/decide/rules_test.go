package decide

import (
	"context"
	"testing"
)

func TestRulesAlwaysEnabled(t *testing.T) {
	r := NewRules()
	if !r.Enabled() {
		t.Fatal("rules must always be enabled")
	}
}

func TestRulesMemoryShortChitchatIsNo(t *testing.T) {
	r := NewRules()
	ans, err := r.Ask(context.Background(), Question{
		Kind:    KindMemoryExtract,
		Context: "用户输入：你好\n助手回复：你好呀",
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if ans.Verdict != VerdictNo || ans.Source != SourceRules {
		t.Fatalf("ans=%+v want no/rules", ans)
	}
}

func TestRulesMemoryFactualOrLongIsYes(t *testing.T) {
	r := NewRules()
	cases := []string{
		"用户输入：记住我的工号是 88021\n助手回复：好的，我记住了你的工号是88021。",
		"用户输入：请分析这个方案并给出详细改进建议\n助手回复：" +
			"这是一段足够长的回复内容，用于触发长度规则，确保不会被误判为无事实。",
	}
	for _, ctxStr := range cases {
		ans, err := r.Ask(context.Background(), Question{Kind: KindMemoryExtract, Context: ctxStr})
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if ans.Verdict != VerdictYes {
			t.Fatalf("ctx=%q verdict=%v want yes", ctxStr, ans.Verdict)
		}
	}
}

func TestRulesUnknownKindAbstains(t *testing.T) {
	r := NewRules()
	_, err := r.Ask(context.Background(), Question{Kind: "something_else"})
	if err != ErrUnavailable {
		t.Fatalf("err=%v want ErrUnavailable", err)
	}
}
