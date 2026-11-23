package conversation_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/conversation"
)

func TestSplitGoalTitleAndDetailsShort(t *testing.T) {
	title, details := conversation.SplitGoalTitleAndDetails("修好登录")
	if title != "修好登录" || details != "" {
		t.Fatalf("title=%q details=%q", title, details)
	}
}

func TestSplitGoalTitleAndDetailsAtSentence(t *testing.T) {
	raw := "修好登录页报错。" + strings.Repeat("补充背景说明。", 20)
	title, details := conversation.SplitGoalTitleAndDetails(raw)
	if title != "修好登录页报错。" {
		t.Fatalf("title=%q", title)
	}
	if details == "" || strings.Contains(details, "修好登录页报错") {
		t.Fatalf("details=%q", details)
	}
	if utf8.RuneCountInString(title) > conversation.SoftMaxGoalTitleRunes {
		t.Fatalf("title too long: %d", utf8.RuneCountInString(title))
	}
}

func TestSetRootDetailedPersistsDetails(t *testing.T) {
	s, err := conversation.SetRootDetailed(
		conversation.GoalStack{}, "c1", "短目标", "很长的约束与背景", nil, conversation.GoalSourceAgent,
	)
	if err != nil {
		t.Fatal(err)
	}
	if s.Frames[0].Text != "短目标" || s.Frames[0].Details != "很长的约束与背景" {
		t.Fatalf("frame=%+v", s.Frames[0])
	}
}
