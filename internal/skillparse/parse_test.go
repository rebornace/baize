package skillparse_test

import (
	"reflect"
	"testing"

	"github.com/rebornace/baize/internal/skillparse"
)

func TestParseMentions(t *testing.T) {
	cleaned, ids := skillparse.Parse(`请用 @data-analytics 和 /ticket-triage 分析`)
	if cleaned != "请用 和 分析" {
		t.Fatalf("cleaned = %q, want %q", cleaned, "请用 和 分析")
	}
	want := []string{"data-analytics", "ticket-triage"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
}

func TestParseNoMention(t *testing.T) {
	cleaned, ids := skillparse.Parse("普通问题")
	if cleaned != "普通问题" || len(ids) != 0 {
		t.Fatal(cleaned, ids)
	}
}

func TestParseDedupPreservesOrder(t *testing.T) {
	_, ids := skillparse.Parse("@b @a @b @c")
	want := []string{"b", "a", "c"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
}

func TestParseAtStart(t *testing.T) {
	cleaned, ids := skillparse.Parse("@data-analytics 分析数据")
	if cleaned != "分析数据" {
		t.Fatalf("cleaned = %q, want %q", cleaned, "分析数据")
	}
	if !reflect.DeepEqual(ids, []string{"data-analytics"}) {
		t.Fatalf("ids = %v", ids)
	}
}

func TestParseIgnoresInvalidMentions(t *testing.T) {
	cleaned, ids := skillparse.Parse("foo@-invalid and @valid thing")
	if cleaned != "foo@-invalid and thing" {
		t.Fatalf("cleaned = %q, want %q", cleaned, "foo@-invalid and thing")
	}
	if !reflect.DeepEqual(ids, []string{"valid"}) {
		t.Fatalf("ids = %v", ids)
	}
}

func TestParseCollapsesNewlines(t *testing.T) {
	cleaned, ids := skillparse.Parse("line1\n@skill\nline2")
	if cleaned != "line1 line2" {
		t.Fatalf("cleaned = %q, want %q", cleaned, "line1 line2")
	}
	if !reflect.DeepEqual(ids, []string{"skill"}) {
		t.Fatalf("ids = %v", ids)
	}
}
