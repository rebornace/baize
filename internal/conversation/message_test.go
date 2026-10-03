package conversation

import "testing"

func TestStripMediaRefs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "plain text unchanged",
			in:   "帮我总结一下",
			want: "帮我总结一下",
		},
		{
			name: "image marker removed, text kept",
			in:   "看这张图\n![图片](/v0/channels/media/c1/abc.png)",
			want: "看这张图",
		},
		{
			name: "file marker removed, text kept",
			in:   "处理这个文件\n[file:行程.docx](/v0/channels/media/c1/uuid.docx)",
			want: "处理这个文件",
		},
		{
			name: "mixed markers in order removed",
			in:   "都看看\n![图片](/v0/channels/media/c1/a.png)\n[file:n.xlsx](/v0/channels/media/c1/b.xlsx)",
			want: "都看看",
		},
		{
			name: "only markers",
			in:   "![图片](/v0/channels/media/c1/a.jpg)",
			want: "",
		},
		{
			name: "crlf tolerated",
			in:   "文本\r\n![图片](/v0/channels/media/c1/a.jpg)\r\n",
			want: "文本",
		},
		{
			name: "external markdown links untouched",
			in:   "看 [官网](https://example.com/page)",
			want: "看 [官网](https://example.com/page)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripMediaRefs(tc.in); got != tc.want {
				t.Fatalf("StripMediaRefs(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
