package run

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/llm"
)

func prefilterSpecs() []llm.ToolSpec {
	return []llm.ToolSpec{
		{Name: "OmsOrderController_list", Description: "查询订单"},
		{Name: "OmsOrderController_detail", Description: "获取订单详情：订单信息"},
		{Name: "PmsSkuStockController_getList", Description: "查询SKU库存"},
		{Name: "UmsMemberLevelController_list", Description: "查询所有会员等级"},
		{Name: "notion-search", Description: "在Notion中搜索文档"},
	}
}

// The order-list tool must rank first for an order-listing request even though
// its English name never literally contains the Chinese words, because the
// keyword overlap hits its Chinese description.
func TestPrefilterRanksOrderToolForChineseQuery(t *testing.T) {
	picked, noMatch := prefilterTools("帮我查询一下最近的订单列表", prefilterSpecs(), 2)
	if noMatch {
		t.Fatalf("unexpected noMatch")
	}
	if len(picked) != 2 {
		t.Fatalf("len=%d want 2", len(picked))
	}
	if picked[0].Name != "OmsOrderController_list" {
		t.Fatalf("top=%q want OmsOrderController_list; picked=%+v", picked[0].Name, picked)
	}
	if picked[1].Name != "OmsOrderController_detail" {
		t.Fatalf("second=%q want OmsOrderController_detail", picked[1].Name)
	}
}

// A query whose keywords match nothing in the catalog yields noMatch, so a
// decision model is not invited to hallucinate a tool.
func TestPrefilterNoMatch(t *testing.T) {
	picked, noMatch := prefilterTools("量子隧穿效应的数学推导", prefilterSpecs(), 3)
	if !noMatch {
		t.Fatalf("want noMatch=true, got picked=%+v", picked)
	}
	if picked != nil {
		t.Fatalf("picked should be nil on noMatch, got %+v", picked)
	}
}

// limit is honored: asking for one returns only the single best tool.
func TestPrefilterRespectsLimit(t *testing.T) {
	picked, _ := prefilterTools("帮我查询一下最近的订单列表", prefilterSpecs(), 1)
	if len(picked) != 1 {
		t.Fatalf("len=%d want 1", len(picked))
	}
	if picked[0].Name != "OmsOrderController_list" {
		t.Fatalf("top=%q want OmsOrderController_list", picked[0].Name)
	}
}

// A Latin identifier token in the query must boost tools whose *name* contains
// it (name weight), ranking both order operations above the rest.
func TestPrefilterNameTokenBoost(t *testing.T) {
	picked, noMatch := prefilterTools("order", prefilterSpecs(), 2)
	if noMatch {
		t.Fatalf("unexpected noMatch")
	}
	if len(picked) != 2 {
		t.Fatalf("len=%d want 2", len(picked))
	}
	if picked[0].Name != "OmsOrderController_list" || picked[1].Name != "OmsOrderController_detail" {
		t.Fatalf("picked=%+v want both order tools", picked)
	}
}

// Repeated calls with the same inputs return identical ordered results.
func TestPrefilterDeterministic(t *testing.T) {
	specs := prefilterSpecs()
	a, _ := prefilterTools("查询订单", specs, 3)
	b, _ := prefilterTools("查询订单", specs, 3)
	if len(a) != len(b) {
		t.Fatalf("length differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			t.Fatalf("order differs at %d: %q vs %q", i, a[i].Name, b[i].Name)
		}
	}
}

// Edge inputs never panic and report no match.
func TestPrefilterEdgeInputs(t *testing.T) {
	if _, noMatch := prefilterTools("订单", nil, 5); !noMatch {
		t.Fatalf("nil specs must be noMatch")
	}
	if _, noMatch := prefilterTools("", prefilterSpecs(), 5); !noMatch {
		t.Fatalf("empty query must be noMatch")
	}
	if _, noMatch := prefilterTools("订单", prefilterSpecs(), 0); !noMatch {
		t.Fatalf("limit<=0 must be noMatch")
	}
}

// A query made entirely of function/filler words ("帮我查询一下") must be
// treated as no match rather than matching every tool that says "查询".
func TestPrefilterAllStopwordsIsNoMatch(t *testing.T) {
	if _, noMatch := prefilterTools("帮我查询一下", prefilterSpecs(), 5); !noMatch {
		t.Fatalf("all-stopword query must be noMatch")
	}
}

// Stopwords must be stripped but a real content word still matches: "查一下
// 订单" keeps only 订单 and still finds the order tool.
func TestPrefilterStopwordStrippedKeepsContent(t *testing.T) {
	picked, noMatch := prefilterTools("查一下订单", prefilterSpecs(), 1)
	if noMatch {
		t.Fatalf("unexpected noMatch")
	}
	if picked[0].Name != "OmsOrderController_list" {
		t.Fatalf("top=%q want OmsOrderController_list", picked[0].Name)
	}
}

// A tool whose description shares many weak words but NOT the rare intent word
// must not outrank one that shares the rare word. Here the order tools match
// 订单; no fabricated weak-word tool should beat them.
func TestPrefilterRareWordBeatsWeakOverlap(t *testing.T) {
	specs := append(prefilterSpecs(), llm.ToolSpec{
		Name:        "NoisyController_list",
		Description: "查询列表信息分页", // only weak/generic words, no 订单
	})
	picked, noMatch := prefilterTools("查一下最近的订单", specs, 2)
	if noMatch {
		t.Fatalf("unexpected noMatch")
	}
	if picked[0].Name != "OmsOrderController_list" {
		t.Fatalf("top=%q want OmsOrderController_list (rare 订单 beats weak overlap)", picked[0].Name)
	}
}

func TestBuildCompactTrajectoryShape(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "忽略"},
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: "tool_a", Arguments: map[string]any{"id": "sku-9"}},
			},
		},
		{Role: llm.RoleTool, ToolCallID: "c1", Content: "查询到结果"},
	}
	out := buildCompactTrajectory(msgs, 800)
	if !strings.Contains(out, "-> tool_a(id=sku-9)") {
		t.Fatalf("missing call line: %q", out)
	}
	if !strings.Contains(out, "<- 查询到结果") {
		t.Fatalf("missing result line: %q", out)
	}
	if strings.Contains(out, "忽略") {
		t.Fatalf("plain prose must not enter trajectory: %q", out)
	}
}

func TestBuildCompactTrajectoryEmptyOnFirstStep(t *testing.T) {
	msgs := []llm.Message{{Role: llm.RoleUser, Content: "你好"}}
	if got := buildCompactTrajectory(msgs, 800); got != "" {
		t.Fatalf("first step trajectory must be empty, got %q", got)
	}
}

func TestBuildCompactTrajectoryTruncatesLongResult(t *testing.T) {
	long := strings.Repeat("数", 500)
	msgs := []llm.Message{
		{
			Role:      llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{ID: "c1", Name: "tool_a"}},
		},
		{Role: llm.RoleTool, ToolCallID: "c1", Content: long},
	}
	out := buildCompactTrajectory(msgs, 800)
	if r := []rune(out); len(r) > 800 {
		t.Fatalf("trajectory must be capped, got %d runes", len(r))
	}
	if strings.Contains(out, strings.Repeat("数", 121)) {
		t.Fatalf("individual result must be truncated to item limit")
	}
}

// Multi-system quota: when tools come from two connectors and the total limit
// is small, the round-robin merge must give BOTH systems a share instead of
// letting one system flood the flat top-K. Each system also needs its own
// discriminative content word so its tool ranks within that group.
func TestPrefilterBySystemSharesSlotsAcrossSystems(t *testing.T) {
	specs := []llm.ToolSpec{
		{Name: "OmsOrderController_list", Description: "查询订单", Source: "mall"},
		{Name: "OmsOrderController_detail", Description: "订单详情", Source: "mall"},
		{Name: "PetsAdminController_findPage", Description: "列出宠物", Source: "doctor-miao"},
		{Name: "PetsAdminController_findOne", Description: "宠物详情", Source: "doctor-miao"},
	}
	allowed := map[string]bool{"mall": true, "doctor-miao": true}
	picked, noMatch := prefilterToolsBySystem("订单和宠物", specs, allowed, 4)
	if noMatch {
		t.Fatalf("unexpected noMatch")
	}
	sources := make(map[string]int)
	for _, s := range picked {
		sources[s.Source]++
	}
	if sources["mall"] == 0 || sources["doctor-miao"] == 0 {
		t.Fatalf("both systems must get a share, got sources=%v picked=%+v", sources, picked)
	}
}

// buildSystemDescriptions must surface a system's own discriminative domain
// words (宠物 for doctor-miao, 订单 for mall) so the routing model is not
// choosing from meaningless bare ids, and shared filler words must not dominate.
func TestBuildSystemDescriptions(t *testing.T) {
	specs := []llm.ToolSpec{
		{Name: "PetsAdminController_findPage", Description: "列出宠物", Source: "doctor-miao"},
		{Name: "FamiliesAdminController_findPage", Description: "家庭列表", Source: "doctor-miao"},
		{Name: "OmsOrderController_list", Description: "查询订单", Source: "mall"},
		{Name: "OmsOrderController_detail", Description: "订单详情", Source: "mall"},
	}
	d := buildSystemDescriptions(specs, []string{"doctor-miao", "mall"})
	if !strings.Contains(d["doctor-miao"], "宠物") {
		t.Fatalf("doctor-miao desc should contain 宠物, got %q", d["doctor-miao"])
	}
	if !strings.Contains(d["mall"], "订单") {
		t.Fatalf("mall desc should contain 订单, got %q", d["mall"])
	}
}
func TestPrefilterBySystemRespectsAllowed(t *testing.T) {
	specs := []llm.ToolSpec{
		{Name: "OmsOrderController_list", Description: "查询订单", Source: "mall"},
		{Name: "PetsAdminController_findPage", Description: "列出宠物", Source: "doctor-miao"},
	}
	picked, noMatch := prefilterToolsBySystem("订单和宠物", specs, map[string]bool{"mall": true}, 5)
	if noMatch {
		t.Fatalf("unexpected noMatch")
	}
	for _, s := range picked {
		if s.Source == "doctor-miao" {
			t.Fatalf("non-selected system tool must be excluded, got %+v", picked)
		}
	}
}
