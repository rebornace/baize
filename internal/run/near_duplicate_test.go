package run

import "testing"

func nearDupPair(a, b string) bool {
	return newNearDupText(a).similarTo(newNearDupText(b))
}

func TestNearDuplicateDogfoodPair(t *testing.T) {
	a := "统计问卷时默认按完成人数汇报，不按配额进度；输出中文表格。"
	b := "统计问卷默认按「完成人数」口径汇报，不按配额进度，输出中文表格。"
	if !nearDupPair(a, b) {
		t.Fatal("dogfood restatement must be near-duplicate")
	}
}

func TestNearDuplicateRejectsDifferentLanguage(t *testing.T) {
	if nearDupPair("统计问卷输出中文表格", "统计问卷输出英文表格") {
		t.Fatal("zh vs en must not be near-duplicate")
	}
}

func TestNearDuplicateRejectsDifferentNumbers(t *testing.T) {
	cases := [][2]string{
		{"链接有效期30天", "链接有效期7天"},
		{"问卷12分配给用户18", "问卷12分配给用户20"},
		{"问卷11共22题", "问卷11共10道追问题"},
	}
	for _, c := range cases {
		if nearDupPair(c[0], c[1]) {
			t.Fatalf("different numbers must not match: %q vs %q", c[0], c[1])
		}
	}
}

func TestNearDuplicateRejectsLengthRatio(t *testing.T) {
	long := "统计问卷默认按完成人数汇报并且输出中文表格并附带关键选项分布与样本质量说明"
	short := "统计问卷按完成人数"
	if nearDupPair(long, short) {
		t.Fatal("extreme length ratio must not match")
	}
}

func TestNearDuplicateRejectsShortOneCharacterSwap(t *testing.T) {
	if nearDupPair("喜欢绿茶", "喜欢红茶") {
		t.Fatal("one-character swap in a short fact must not match")
	}
}
