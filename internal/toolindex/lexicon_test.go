package toolindex

import (
	"testing"
)

func TestIntentFloorsBilingual(t *testing.T) {
	for _, q := range []string{"删除用户", "please delete user 116", "删用户"} {
		if !HasDestructiveIntent(q) {
			t.Fatalf("%q: want destructive intent (preserve floor only)", q)
		}
	}
	for _, q := range []string{"列出订单", "show me the orders", "how many pets", "top 5 orders", "page 2 of users"} {
		if !HasListIntent(q) {
			t.Fatalf("%q: want list intent", q)
		}
	}
}

func TestEnglishFillerStopwords(t *testing.T) {
	toks := RefineQueryTokens(Tokens("please help me show the orders"))
	for _, bad := range []string{"please", "help", "me", "the"} {
		if toks[bad] {
			t.Fatalf("filler %q should be stripped, got %v", bad, toks)
		}
	}
	if !toks["orders"] && !toks["order"] && !toks["show"] {
		t.Fatalf("want content tokens kept, got %v", toks)
	}
}

func TestQueryTokensNoVerbExpand(t *testing.T) {
	// Retrieval must not inject Latin delete/remove from Chinese — that is
	// the Embedder's job. Bare lexical tokens only.
	toks := QueryTokens("删用户")
	if toks["delete"] || toks["remove"] {
		t.Fatalf("must not synonym-expand verbs, got %v", toks)
	}
}

func TestDestroyOpToken(t *testing.T) {
	for _, op := range []string{"delete", "remove", "destroy", "erase"} {
		if !IsDestroyOpToken(op) {
			t.Fatalf("want destroy op %q", op)
		}
	}
	if IsDestroyOpToken("list") {
		t.Fatal("list is not destroy")
	}
}
