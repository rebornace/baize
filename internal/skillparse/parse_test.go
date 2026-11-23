package skillparse_test

import (
	"reflect"
	"testing"

	"github.com/rebornace/baize/internal/skillparse"
)

func TestParseMentions(t *testing.T) {
	r := skillparse.Parse(`请用 @data-analytics 和 /ticket-triage 分析`)
	if r.Cleaned != "请用 和 分析" {
		t.Fatalf("cleaned = %q, want %q", r.Cleaned, "请用 和 分析")
	}
	want := []string{"data-analytics", "ticket-triage"}
	if !reflect.DeepEqual(r.IDs, want) {
		t.Fatalf("ids = %v, want %v", r.IDs, want)
	}
	if r.Reload {
		t.Fatal("expected Reload=false")
	}
}

func TestParseNoMention(t *testing.T) {
	r := skillparse.Parse("普通问题")
	if r.Cleaned != "普通问题" || len(r.IDs) != 0 || r.Reload {
		t.Fatal(r)
	}
}

func TestParseDedupPreservesOrder(t *testing.T) {
	r := skillparse.Parse("@b @a @b @c")
	want := []string{"b", "a", "c"}
	if !reflect.DeepEqual(r.IDs, want) {
		t.Fatalf("ids = %v, want %v", r.IDs, want)
	}
}

func TestParseMentionOnly(t *testing.T) {
	r := skillparse.Parse("@login-crm")
	if r.Cleaned != "" {
		t.Fatalf("cleaned = %q, want empty", r.Cleaned)
	}
	if !reflect.DeepEqual(r.IDs, []string{"login-crm"}) {
		t.Fatalf("ids = %v", r.IDs)
	}
	if !skillparse.IsMentionOnly("@login-crm ") {
		t.Fatal("expected mention-only")
	}
	if skillparse.IsMentionOnly("@login-crm 帮我登录") {
		t.Fatal("text plus mention is not mention-only")
	}
}

func TestParseAtStart(t *testing.T) {
	r := skillparse.Parse("@data-analytics 分析数据")
	if r.Cleaned != "分析数据" {
		t.Fatalf("cleaned = %q, want %q", r.Cleaned, "分析数据")
	}
	if !reflect.DeepEqual(r.IDs, []string{"data-analytics"}) {
		t.Fatalf("ids = %v", r.IDs)
	}
}

func TestParseIgnoresInvalidMentions(t *testing.T) {
	r := skillparse.Parse("foo@-invalid and @valid thing")
	if r.Cleaned != "foo@-invalid and thing" {
		t.Fatalf("cleaned = %q, want %q", r.Cleaned, "foo@-invalid and thing")
	}
	if !reflect.DeepEqual(r.IDs, []string{"valid"}) {
		t.Fatalf("ids = %v", r.IDs)
	}
}

func TestParseCollapsesNewlines(t *testing.T) {
	r := skillparse.Parse("line1\n@skill\nline2")
	if r.Cleaned != "line1 line2" {
		t.Fatalf("cleaned = %q, want %q", r.Cleaned, "line1 line2")
	}
	if !reflect.DeepEqual(r.IDs, []string{"skill"}) {
		t.Fatalf("ids = %v", r.IDs)
	}
}

func TestParseReloadToken(t *testing.T) {
	r := skillparse.Parse("/reload")
	if r.Cleaned != "" || len(r.IDs) != 0 || !r.Reload {
		t.Fatalf("got %+v", r)
	}
	if !skillparse.IsMentionOnly("@reload") {
		t.Fatal("reload-only should be mention-only")
	}

	r = skillparse.Parse("@ticket-triage /reload please")
	if r.Cleaned != "please" {
		t.Fatalf("cleaned = %q", r.Cleaned)
	}
	if !reflect.DeepEqual(r.IDs, []string{"ticket-triage"}) {
		t.Fatalf("ids = %v", r.IDs)
	}
	if !r.Reload {
		t.Fatal("expected Reload")
	}
	// reload must not appear as a skill id even when repeated
	r = skillparse.Parse("@reload /reload @reload")
	if len(r.IDs) != 0 || !r.Reload {
		t.Fatalf("got %+v", r)
	}
}
