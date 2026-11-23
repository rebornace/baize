package run

import (
	"strings"
	"testing"
)

func TestUnverifiedWriteClaims(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{"zh copy done", "复制完成，新问卷 id 是 13。", true},
		{"zh copied", "已把问卷 11 复制到团队 11。", true},
		{"zh created", "已创建问卷 15。", true},
		{"zh deleted", "已删除问卷 15。", true},
		{"zh negated", "没有复制，因为权限不足。", false},
		{"zh failed", "复制失败：接口返回 500。", false},
		{"zh read", "已查询到问卷 11。", false},
		{"zh read done", "已完成查询。", false},
		{"zh found", "已找到 3 条记录。", false},
		{"zh done only", "已完成。", false},
		{"zh question", "是否已复制到团队 11？", false},
		{"en created", "I have created questionnaire 15.", true},
		{"en copied", "Copied successfully.", true},
		{"en cannot", "I could not create it.", false},
		{"en not", "I have not created it.", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := unverifiedWriteClaims(tc.output)
			if tc.want && len(got) == 0 {
				t.Fatalf("want a claim for %q, got none", tc.output)
			}
			if !tc.want && len(got) > 0 {
				t.Fatalf("want no claim for %q, got %v", tc.output, got)
			}
		})
	}
}

func TestUnverifiedWriteClaimsDedupesAndCaps(t *testing.T) {
	out := strings.Repeat("已复制问卷 11。", 8)
	got := unverifiedWriteClaims(out)
	if len(got) == 0 {
		t.Fatal("want claims")
	}
	if len(got) > 5 {
		t.Fatalf("claims=%d want <=5", len(got))
	}
	seen := map[string]bool{}
	for _, c := range got {
		if len([]rune(c)) > 120 {
			t.Fatalf("claim too long: %q", c)
		}
		if seen[c] {
			t.Fatalf("duplicate claim: %q", c)
		}
		seen[c] = true
	}
}
