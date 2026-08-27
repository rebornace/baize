package workflow_test

import (
	"reflect"
	"testing"

	"github.com/rebornace/baize/internal/workflow"
)

func tree() map[string]any {
	return map[string]any{
		"input": map[string]any{"text": "printer on fire"},
		"fetch": map[string]any{"result": map[string]any{
			"summary": "fire report",
			"urgent":  true,
			"count":   float64(3),
			"items":   []any{map[string]any{"id": "t1"}, map[string]any{"id": "t2"}},
			"meta":    map[string]any{"owner": map[string]any{"name": "bob"}},
		}},
	}
}

func TestRenderWholeValueKeepsType(t *testing.T) {
	tree := tree()
	got, ok := workflow.RenderArg("{{fetch.result.urgent}}", tree)
	if !ok || got != true {
		t.Fatalf("got=%v ok=%v", got, ok)
	}
	got, _ = workflow.RenderArg("{{fetch.result.count}}", tree)
	if got != float64(3) {
		t.Fatalf("got=%T %v", got, got)
	}
}

func TestRenderSubstringConcatenates(t *testing.T) {
	got, _ := workflow.RenderArg("ticket: {{fetch.result.count}}!", tree())
	if got != "ticket: 3!" {
		t.Fatalf("got=%#v", got)
	}
}

func TestRenderInputText(t *testing.T) {
	got, _ := workflow.RenderArg("{{input.text}}", tree())
	if got != "printer on fire" {
		t.Fatalf("got=%v", got)
	}
}

func TestRenderNestedAndIndex(t *testing.T) {
	got, _ := workflow.RenderArg("{{fetch.result.items.1.id}}", tree())
	if got != "t2" {
		t.Fatalf("got=%v", got)
	}
}

func TestRenderMissingPathIsError(t *testing.T) {
	_, ok := workflow.RenderArg("{{fetch.result.summary.x}}", tree())
	if ok {
		t.Fatal("want not-found")
	}
	_, ok = workflow.RenderArg("{{nope.result}}", tree())
	if ok {
		t.Fatal("want not-found")
	}
}

func TestRenderMapRecursive(t *testing.T) {
	in := map[string]any{
		"a": "{{fetch.result.summary}}",
		"b": []any{"{{input.text}}", 7},
	}
	got := workflow.RenderArgs(in, tree())
	want := map[string]any{"a": "fire report", "b": []any{"printer on fire", 7}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%#v", got)
	}
}

func TestPlaceholderRegexOnlyMatchesFullOrPart(t *testing.T) {
	cases := map[string]string{
		"{{fetch.result.summary}}x": "fire reportx", // 子串拼接
	}
	for in, want := range cases {
		got, _ := workflow.RenderArg(in, tree())
		if got != want {
			t.Fatalf("%s => %#v", in, got)
		}
	}
}
