package run

import (
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
