package toolindex

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/llm"
)

// conceptEmbedder is test-only: maps shared concepts into the same dense
// dimensions so ZH/EN paraphrases retrieve without product synonym tables.
type conceptEmbedder struct{}

func (conceptEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	const dim = 8
	out := make([][]float32, len(texts))
	for i, text := range texts {
		v := make([]float32, dim)
		lower := strings.ToLower(text)
		if strings.Contains(lower, "delete") || strings.Contains(lower, "remove") ||
			strings.Contains(text, "删") || strings.Contains(text, "移除") {
			v[0] = 1
		}
		if strings.Contains(lower, "user") || strings.Contains(text, "用户") {
			v[1] = 1
		}
		if strings.Contains(lower, "order") || strings.Contains(text, "订单") {
			v[2] = 1
		}
		if strings.Contains(lower, "login") || strings.Contains(text, "登录") {
			v[3] = 1
		}
		if strings.Contains(lower, "list") || strings.Contains(text, "列表") || strings.Contains(text, "分页") {
			v[4] = 1
		}
		out[i] = l2normalize(v)
	}
	return out, nil
}

func TestDenseRetrieveDeleteUserCrossLingual(t *testing.T) {
	specs := []llm.ToolSpec{
		{Name: "UsersAdminController_login", Description: "用户登录", Source: "pets-admin", Method: "POST", Path: "/admin/login"},
		{Name: "UsersAdminController_remove", Description: "移除用户记录", Source: "pets-admin", Method: "POST", Path: "/admin/users/{id}/remove"},
		{Name: "OmsOrderController_list", Description: "查询订单", Source: "mall", Method: "GET", Path: "/orders"},
	}
	emb := conceptEmbedder{}
	idx, err := BuildDense(context.Background(), specs, emb)
	if err != nil {
		t.Fatal(err)
	}
	if idx.mode != ModeDenseHybrid {
		t.Fatalf("mode=%q", idx.mode)
	}
	for _, q := range []string{"删除用户", "please delete user 116", "后台删用户id为116"} {
		res := idx.Retrieve(context.Background(), q, Options{
			Limit:            2,
			PreferredSources: map[string]bool{"mall": true},
			WidenIfSparse:    true,
		}, emb)
		if res.Empty {
			t.Fatalf("%q: empty", q)
		}
		if res.Hits[0].Spec.Name != "UsersAdminController_remove" {
			t.Fatalf("%q: top=%q want remove (dense must beat wrong preferred=mall)", q, res.Hits[0].Spec.Name)
		}
	}
}

func TestLexicalRetrieveOrderQuery(t *testing.T) {
	specs := []llm.ToolSpec{
		{Name: "OmsOrderController_list", Description: "查询订单列表", Source: "mall"},
		{Name: "PetsAdminController_list", Description: "查询宠物列表", Source: "pets"},
		{Name: "UsersAdminController_findPage", Description: "用户分页", Source: "pets-admin"},
	}
	res := Build(specs).RetrieveLexical("帮我查询一下最近的订单列表", Options{Limit: 2})
	if res.Empty {
		t.Fatal("empty")
	}
	if res.Hits[0].Spec.Name != "OmsOrderController_list" {
		t.Fatalf("top=%q want OmsOrderController_list", res.Hits[0].Spec.Name)
	}
	if res.Mode != ModeLexical {
		t.Fatalf("mode=%q want %s", res.Mode, ModeLexical)
	}
}

func TestRetrieveNoMatch(t *testing.T) {
	specs := []llm.ToolSpec{
		{Name: "OmsOrderController_list", Description: "查询订单"},
	}
	res := Build(specs).RetrieveLexical("量子隧穿效应的数学推导", Options{Limit: 3})
	if !res.Empty {
		t.Fatalf("want empty, got %v", SpecsFromHits(res.Hits))
	}
}

func TestRetrievePathTokensHelpEnglishOps(t *testing.T) {
	specs := []llm.ToolSpec{
		{Name: "ctrl_a", Description: "manage records", Method: "DELETE", Path: "/api/users/{id}", Source: "a"},
		{Name: "ctrl_b", Description: "manage records", Method: "GET", Path: "/api/orders", Source: "b"},
	}
	res := Build(specs).RetrieveLexical("delete users", Options{Limit: 1})
	if res.Empty {
		t.Fatal("empty")
	}
	if res.Hits[0].Spec.Name != "ctrl_a" {
		t.Fatalf("top=%q want ctrl_a via path token users", res.Hits[0].Spec.Name)
	}
}

func TestCatalogFingerprintStable(t *testing.T) {
	specs := []llm.ToolSpec{{Name: "a", Description: "x"}}
	a := CatalogFingerprint(specs)
	b := CatalogFingerprint(specs)
	if a == "" || a != b {
		t.Fatal("fingerprint must be stable and non-empty")
	}
	other := CatalogFingerprint([]llm.ToolSpec{{Name: "b", Description: "y"}})
	if a == other {
		t.Fatal("different catalogs must fingerprint differently")
	}
}
