package workspace

import "testing"

func TestSafeRelPath(t *testing.T) {
	ok := []struct{ in, want string }{
		{"uploads/a.txt", "uploads/a.txt"},
		{"notes/summary.md", "notes/summary.md"},
		{"./a.txt", "a.txt"},
		{"a//b", "a/b"},
		{"a/./b", "a/b"},
		{"a/b/../c", "a/c"},
		{"uploads\\report.pdf", "uploads/report.pdf"}, // Windows 分隔符归一
	}
	for _, c := range ok {
		got, err := safeRelPath(c.in)
		if err != nil {
			t.Fatalf("safeRelPath(%q) unexpected err %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("safeRelPath(%q)=%q want %q", c.in, got, c.want)
		}
	}

	bad := []string{
		"", ".", "/abs/path", "../x", "..", "a/../../b", "C:/x", "c:\\x", "uploads/../../../etc",
		// Backslash traversal and UNC/drive paths must be rejected on every OS
		// (backslash is unconditionally normalized to "/" before cleaning).
		"..\\..\\x",
		"a\\..\\..\\b",
		"\\\\server\\share",
		"./..",
		"foo/../..",
		"//server/share",
		"C:",
	}
	for _, in := range bad {
		if _, err := safeRelPath(in); err == nil {
			t.Fatalf("safeRelPath(%q) should error", in)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"report.pdf":       "report.pdf",
		"../../etc/passwd": "passwd",
		"a/b\\c.txt":       "c.txt",
		"my report (1).md": "my report (1).md",
		"bad\x00name.txt":  "badname.txt",
		"..":               "file",
		"":                 "file",
		".":                "file",
		"/":                "file",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Fatalf("sanitizeName(%q)=%q want %q", in, got, want)
		}
	}
}
