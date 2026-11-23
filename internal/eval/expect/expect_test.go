package expect_test

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/rebornace/baize/internal/eval/expect"
)

func TestResolveNestedAndArrayPaths(t *testing.T) {
	record := map[string]any{
		"id":    "p-1",
		"title": "Alpha",
		"meta": map[string]any{
			"owner": map[string]any{"id": "u-9"},
		},
		"tags": []any{"urgent", "infra"},
		"items": []any{
			map[string]any{"name": "first"},
			map[string]any{"name": "second"},
		},
	}
	cases := []struct {
		path string
		want string
	}{
		{"id", "p-1"},
		{"meta.owner.id", "u-9"},
		{"tags.1", "infra"},
		{"items.0.name", "first"},
	}
	for _, tc := range cases {
		got, ok := expect.Resolve(record, tc.path)
		if !ok || expect.ValueString(got) != tc.want {
			t.Fatalf("%s: got=%v ok=%v want=%s", tc.path, got, ok, tc.want)
		}
	}
	if _, ok := expect.Resolve(record, "tags.9"); ok {
		t.Fatal("out-of-range index must miss")
	}
	if _, ok := expect.Resolve(record, "meta.owner.missing"); ok {
		t.Fatal("missing key must miss")
	}
}

func TestMatchOps(t *testing.T) {
	if !expect.Match("Alpha project", expect.Field{Op: "contains", Value: "Alpha"}) {
		t.Fatal("contains")
	}
	if expect.Match("Beta", expect.Field{Op: "eq", Value: "Alpha"}) {
		t.Fatal("eq must be exact")
	}
	if !expect.Match("closed", expect.Field{Value: "closed"}) {
		t.Fatal("default op must be eq")
	}
	if !expect.Match("p-42", expect.Field{Op: "regex", Value: `^p-\d+$`}) {
		t.Fatal("regex")
	}
	if expect.Match("p-x", expect.Field{Op: "regex", Value: `^p-\d+$`}) {
		t.Fatal("regex miss")
	}
	if expect.MatchAll(map[string]any{"id": "p1"}, nil) {
		t.Fatal("empty expectation must not match")
	}
}

func TestJSONAndYAMLForms(t *testing.T) {
	var fields map[string]expect.Field
	if err := json.Unmarshal([]byte(`{"status":"open","title":{"op":"contains","value":"bug"}}`), &fields); err != nil {
		t.Fatal(err)
	}
	if fields["status"].EffectiveOp() != "eq" || fields["status"].Value != "open" {
		t.Fatalf("bare scalar: %+v", fields["status"])
	}
	if fields["title"].EffectiveOp() != "contains" {
		t.Fatalf("object: %+v", fields["title"])
	}
	out, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"op":"eq"`) {
		t.Fatalf("normalized json=%s", out)
	}

	var doc struct {
		Fields map[string]expect.Field `yaml:"fields"`
	}
	if err := yaml.Unmarshal([]byte("fields:\n  status: open\n  title:\n    op: regex\n    value: \"^A\"\n"), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Fields["status"].Value != "open" || doc.Fields["title"].EffectiveOp() != "regex" {
		t.Fatalf("yaml fields=%+v", doc.Fields)
	}
}

func TestValidateFields(t *testing.T) {
	if err := expect.ValidateFields(map[string]expect.Field{
		"meta.owner.id": {Value: "u-1"},
		"tags.0":        {Value: "urgent"},
	}); err != nil {
		t.Fatal(err)
	}
	bad := []map[string]expect.Field{
		{},
		{"a..b": {Value: "x"}},
		{"a": {Op: "gt", Value: 1}},
		{"a": {Op: "contains", Value: 1}},
		{"a": {Op: "regex", Value: "("}},
		{"a": {}},
	}
	for i, fields := range bad {
		if err := expect.ValidateFields(fields); err == nil {
			t.Fatalf("case %d must fail: %+v", i, fields)
		}
	}
	if err := expect.ValidateFields(map[string]expect.Field{"a": {Op: "contains", Value: "x"}}); err != nil {
		t.Fatal(err)
	}
}
