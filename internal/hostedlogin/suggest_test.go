package hostedlogin_test

import (
	"testing"

	"github.com/rebornace/baize/internal/hostedlogin"
)

func TestParseSuggestResponse(t *testing.T) {
	raw := []byte(`{"suggestions":[{"id":"tool-mall.login","label":"商城账号登录","recommend_default":true}]}`)
	out, err := hostedlogin.ParseSuggestResponse(raw)
	if err != nil || out[0].Label != "商城账号登录" {
		t.Fatal(err, out)
	}
}
