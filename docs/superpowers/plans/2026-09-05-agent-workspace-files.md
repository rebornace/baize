# Agent 会话文件工作区（Agent Workspace Files）实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为 Agent 提供按会话隔离的持久文件工作区——文本/图片附件持久化到 blob，Agent 通过 `list_files`/`read_file`/`write_file`/`delete_file`/`read_image` 五个内置工具在会话工作区内读写文件、按需看图，跨轮次/跨会话持久。

**架构：** 复用已交付的 `blob.Store`（file/memory/s3）。给 `blob.Store` 增加 `List(ctx, prefix)`；新增 `internal/workspace` 包（路径安全 + 服务 + 5 工具）；新增 `tool.WithImageParts`/`ExtractImageParts` 多模态工具结果通道，引擎两处 Invoke 后把图片 part 编码进 tool 消息，ToolResult 事件剥离图片字节改存 `image_refs`，冷恢复经 `Engine.ImagePartResolver` 从 blob 重取并文本降级；api 发送时把文本/图片附件落工作区并注入清单；bootstrap 用同一 blobStore 装配。

**技术栈：** Go 1.2x、标准库（`net/http`、`path`、`path/filepath`、`io/fs`）、`github.com/minio/minio-go/v7`（s3 驱动，Apache-2.0）、httptest（s3 mock）。

**设计规格：** `docs/superpowers/specs/2026-09-04-agent-workspace-files-design.md`

---

## 文件结构

**新增：**
- `internal/workspace/safe.go` — 逻辑相对路径净化 `safeRelPath` 与文件名净化 `sanitizeName`（纯函数，无依赖）。
- `internal/workspace/safe_test.go`
- `internal/workspace/workspace.go` — `Service`：封装 `blob.Store`，key 前缀 `workspaces/<conv>/`，提供 `SaveUpload`/`SaveUploadBytes`/`ListFiles`/`ReadFile`/`WriteFile`/`DeleteFile`/`ReadImage`/`ResolveImagePart`，常量上限与选项。
- `internal/workspace/service_test.go` — 用 memory blob 驱动测服务往返/隔离/上限。
- `internal/workspace/tools.go` — 5 个 `llm.ToolSpec` + 5 个 `tool.Invoker`（`Tools()` 返回注册用切片）。
- `internal/workspace/tools_test.go`
- `internal/tool/parts.go` — `WithImageParts`/`ExtractImageParts` 保留键助手。
- `internal/tool/parts_test.go`

**修改：**
- `internal/blob/blob.go` — 加 `ListEntry` 与 `Store.List`。
- `internal/blob/file/file.go`、`internal/blob/memory/memory.go`、`internal/blob/s3/s3.go` — 实现 `List`。
- `internal/blob/s3/s3_test.go` — 加 ListObjects mock 分支与测试。
- `internal/run/engine.go` — `Engine` 加 `ImagePartResolver` 字段；两处 Invoke 后提取图片 part 构造 Parts 消息；ToolResult 事件剥离图片、存 `image_refs`；`eventsAfterInput` 经 resolver 重建图片。
- `internal/run/engine_workspace_test.go`（新增测试文件）— 多模态工具结果通道、事件剥离、冷恢复重取/降级。
- `internal/api/server.go` — `Server` 加 `Workspace` 字段（接口类型）；聊天发送处理器落文本+图片附件、注入清单。
- `internal/api/server_workspace_test.go`（新增）— 附件落盘、失败不阻断。
- `internal/bootstrap/bootstrap.go` — 构造 `workspace.Service`、注册 5 工具、注入 `srv.Workspace` 与 `engine.ImagePartResolver`、视觉门控。

**编译顺序要点：** 任务 1 给 `blob.Store` 接口加方法会**立即**让 file/memory/s3 三驱动编译失败，因此任务 1 一次性改完接口 + 三驱动 + 测试，保持每步可编译。任务 4（tool 助手）独立无依赖。任务 2（workspace 包）依赖任务 1 的 `List`。任务 5（引擎通道）依赖任务 4。任务 3（api）依赖任务 2 的接口。任务 6（bootstrap）最后接线。

---

### 任务 1：blob.Store 增加 List（接口 + file/memory/s3 三驱动）

**文件：**
- 修改：`internal/blob/blob.go`
- 修改：`internal/blob/file/file.go`
- 修改：`internal/blob/memory/memory.go`
- 修改：`internal/blob/s3/s3.go`
- 测试：`internal/blob/file/file_test.go`、`internal/blob/memory/memory_test.go`、`internal/blob/s3/s3_test.go`

- [ ] **步骤 1：编写失败的测试（file 驱动）**

在 `internal/blob/file/file_test.go`（若不存在则新建，`package file_test` 或沿用既有 package）加入：

```go
func TestFileListByPrefix(t *testing.T) {
	dir := t.TempDir()
	s, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	must := func(key string, data []byte) {
		t.Helper()
		if err := s.Put(ctx, key, data, ""); err != nil {
			t.Fatal(err)
		}
	}
	must("workspaces/c1/uploads/a.txt", []byte("hello"))
	must("workspaces/c1/uploads/nested/b.txt", []byte("world!"))
	must("workspaces/c1/notes.md", []byte("note"))
	must("workspaces/c2/other.txt", []byte("x"))

	got, err := s.List(ctx, "workspaces/c1/")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{
		"workspaces/c1/uploads/a.txt":        5,
		"workspaces/c1/uploads/nested/b.txt": 6,
		"workspaces/c1/notes.md":             4,
	}
	have := map[string]int64{}
	for _, e := range got {
		have[e.Key] = e.Size
	}
	if len(have) != len(want) {
		t.Fatalf("list count=%d want %d (%v)", len(have), len(want), have)
	}
	for k, sz := range want {
		if have[k] != sz {
			t.Fatalf("key %s size=%d want %d", k, have[k], sz)
		}
	}
}

func TestFileListMissingPrefixEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.List(context.Background(), "workspaces/nope/")
	if err != nil {
		t.Fatalf("missing prefix must not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}
```

（若 file 驱动已有测试文件，`import` 合并：`context`、`testing`、`github.com/rebornace/baize/internal/blob`、`_ "github.com/rebornace/baize/internal/blob/file"` 视包名而定；包内测试可直接 `blob.Open` 前确认已 blank-import file 驱动。）

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/blob/file/ -run TestFileList -v`
预期：FAIL，编译错误 `s.List undefined (type blob.Store has no method List)`。

- [ ] **步骤 3：接口加 List 与 ListEntry**

在 `internal/blob/blob.go` 的 `Store` 接口中（`Delete` 方法之后）加入：

```go
	// List enumerates objects whose key starts with prefix (empty prefix =
	// all objects). A prefix with no objects returns an empty slice and nil
	// error (it is not an error). Keys are driver-relative, "/"-separated,
	// and include any configured driver prefix semantics already stripped
	// (i.e. the same key space used by Put/Get/Delete).
	List(ctx context.Context, prefix string) ([]ListEntry, error)
```

并在接口定义上方加入：

```go
// ListEntry is metadata for one object returned by Store.List.
type ListEntry struct {
	Key  string // full key relative to the store root, "/"-separated
	Size int64
}
```

- [ ] **步骤 4：file 驱动实现 List**

在 `internal/blob/file/file.go` 增加 import `"io/fs"`、`"sort"`、`"strings"`（`path/filepath` 已有），并加入：

```go
func (s *store) List(_ context.Context, prefix string) ([]blob.ListEntry, error) {
	base := s.path(prefix)
	var out []blob.ListEntry
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return fs.SkipDir // 前缀目录不存在：视为空
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		rel, rerr := filepath.Rel(s.root, p)
		if rerr != nil {
			return rerr
		}
		out = append(out, blob.ListEntry{Key: filepath.ToSlash(rel), Size: info.Size()})
		return nil
	})
	if err != nil {
		// WalkDir 在根不存在时对根调用 walkFn 带 os.IsNotExist；上面已 SkipDir。
		if os.IsNotExist(err) {
			return []blob.ListEntry{}, nil
		}
		return nil, fmt.Errorf("list %s: %w", prefix, err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
```

- [ ] **步骤 5：memory 驱动实现 List + 测试**

在 `internal/blob/memory/memory.go` 加 import `"sort"`、`"strings"`，并加入：

```go
func (s *store) List(_ context.Context, prefix string) ([]blob.ListEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]blob.ListEntry, 0)
	for k, v := range s.m {
		if strings.HasPrefix(k, prefix) {
			out = append(out, blob.ListEntry{Key: k, Size: int64(len(v))})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
```

在 `internal/blob/memory/memory_test.go`（无则新建）加入：

```go
func TestMemoryListFiltersByPrefix(t *testing.T) {
	s, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = s.Put(ctx, "workspaces/c1/a.txt", []byte("ab"), "")
	_ = s.Put(ctx, "workspaces/c1/b.txt", []byte("cde"), "")
	_ = s.Put(ctx, "workspaces/c2/z.txt", []byte("z"), "")
	got, err := s.List(ctx, "workspaces/c1/")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2, got %v", got)
	}
	for _, e := range got {
		if !strings.HasPrefix(e.Key, "workspaces/c1/") {
			t.Fatalf("key leaked across prefix: %s", e.Key)
		}
	}
}
```

（memory 测试需 blank-import `_ "github.com/rebornace/baize/internal/blob/memory"`。）

- [ ] **步骤 6：s3 驱动实现 List**

在 `internal/blob/s3/s3.go` 加入：

```go
func (s *store) List(ctx context.Context, prefix string) ([]blob.ListEntry, error) {
	fullPrefix := s.object(prefix)
	out := make([]blob.ListEntry, 0)
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    fullPrefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("s3 list %s: %w", prefix, obj.Err)
		}
		key := obj.Key
		if s.prefix != "" {
			key = strings.TrimPrefix(key, s.prefix+"/")
		}
		out = append(out, blob.ListEntry{Key: key, Size: obj.Size})
	}
	return out, nil
}
```

注意：`minio.ListObjects` 在桶/前缀为空时通过 channel 结束迭代（不报错），空结果自然返回空切片。

- [ ] **步骤 7：s3 mock 支持 ListObjects + 测试**

在 `internal/blob/s3/s3_test.go` 的 `fakeS3.handler` 的 `switch r.Method` 中，`case http.MethodGet:` 分支开头（location 探测之后、对象读取之前）加入列举处理。列举请求形如 `GET /baize?list-type=2&prefix=...`（`object == ""` 且 query 含 `list-type`）：

```go
		if object == "" && r.URL.Query().Get("list-type") == "2" {
			pfx := r.URL.Query().Get("prefix")
			f.mu.Lock()
			type kv struct {
				key  string
				size int64
			}
			var matched []kv
			for k, v := range f.objects {
				if strings.HasPrefix(k, pfx) {
					matched = append(matched, kv{k, int64(len(v))})
				}
			}
			f.mu.Unlock()
			sort.Slice(matched, func(i, j int) bool { return matched[i].key < matched[j].key })
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
			for _, m := range matched {
				fmt.Fprintf(w, `<Contents><Key>%s</Key><Size>%d</Size></Contents>`,
					xmlEscape(m.key), m.size)
			}
			_, _ = io.WriteString(w, `</ListBucketResult>`)
			return
		}
```

在测试文件加 import `"sort"`，并加辅助函数：

```go
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
```

加入测试：

```go
func TestS3ListByPrefix(t *testing.T) {
	fake := newFakeS3()
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer srv.Close()
	s, err := openAgainst(t, srv, "baize", false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = s.Put(ctx, "workspaces/c1/a.txt", []byte("ab"), "")
	_ = s.Put(ctx, "workspaces/c1/n/b.txt", []byte("cde"), "")
	_ = s.Put(ctx, "workspaces/c2/z.txt", []byte("z"), "")
	got, err := s.List(ctx, "workspaces/c1/")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %v", got)
	}
	// key 应已剥离驱动 prefix（"baize/"），与 Put 用的 key 空间一致。
	want := map[string]int64{"workspaces/c1/a.txt": 2, "workspaces/c1/n/b.txt": 3}
	for _, e := range got {
		if want[e.Key] != e.Size {
			t.Fatalf("unexpected entry %+v (want map %v)", e, want)
		}
	}
}
```

- [ ] **步骤 8：运行全部 blob 测试验证通过**

运行：`go test ./internal/blob/... -v`
预期：PASS（file/memory/s3 的 List 与既有 Put/Get/Delete 测试全绿）。

- [ ] **步骤 9：Commit**

```bash
git add internal/blob/
git commit -m "feat(blob): add Store.List with file/memory/s3 implementations"
```

---

### 任务 2：tool 包多模态工具结果助手

**文件：**
- 创建：`internal/tool/parts.go`
- 测试：`internal/tool/parts_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/tool/parts_test.go`：

```go
package tool

import (
	"testing"

	"github.com/rebornace/baize/internal/llm"
)

func TestWithAndExtractImageParts(t *testing.T) {
	content := map[string]any{"path": "uploads/x.png", "bytes": 3}
	res := ImageResult{Path: "uploads/x.png", Part: llm.ContentPart{Type: "image", ImageMIME: "image/png", ImageBytes: []byte("PNGDATA")}}

	with := WithImageParts(content, res)
	if with["path"] != "uploads/x.png" {
		t.Fatalf("original keys must be preserved: %v", with)
	}

	cleaned, results := ExtractImageParts(with)
	if len(results) != 1 || results[0].Path != "uploads/x.png" {
		t.Fatalf("want 1 image result with path, got %v", results)
	}
	if results[0].Part.ImageMIME != "image/png" || string(results[0].Part.ImageBytes) != "PNGDATA" {
		t.Fatalf("image part lost: %+v", results[0])
	}
	if _, leak := cleaned[imagePartsKey]; leak {
		t.Fatalf("reserved key must be stripped from cleaned content: %v", cleaned)
	}
	if cleaned["path"] != "uploads/x.png" {
		t.Fatalf("cleaned content must keep text keys: %v", cleaned)
	}
}

func TestExtractImagePartsNone(t *testing.T) {
	content := map[string]any{"ok": true}
	cleaned, results := ExtractImageParts(content)
	if len(results) != 0 {
		t.Fatalf("want no results, got %v", results)
	}
	if cleaned["ok"] != true {
		t.Fatalf("content must be returned intact: %v", cleaned)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/tool/ -run TestWithAndExtractImageParts -v`
预期：FAIL，`undefined: WithImageParts`。

- [ ] **步骤 3：实现 parts.go**

创建 `internal/tool/parts.go`：

```go
package tool

import "github.com/rebornace/baize/internal/llm"

// imagePartsKey is a reserved content-map key used to carry multimodal image
// results from a tool invoker back to the engine. It is stripped before the
// content is JSON-marshaled into the tool message text or persisted to events.
const imagePartsKey = "__baize_image_parts__"

// ImageResult pairs an image content part with its workspace-relative logical
// path. The engine uses Part to build the multimodal tool message and Path to
// record a lightweight image_refs pointer on the ToolResult event (bytes are
// never persisted to events).
type ImageResult struct {
	Path string
	Part llm.ContentPart
}

// WithImageParts returns a copy of content with image results attached under
// the reserved key. The original text keys are preserved. The engine reads
// them via ExtractImageParts. It does not mutate the input map.
func WithImageParts(content map[string]any, results ...ImageResult) map[string]any {
	out := make(map[string]any, len(content)+1)
	for k, v := range content {
		out[k] = v
	}
	if len(results) > 0 {
		out[imagePartsKey] = results
	}
	return out
}

// ExtractImageParts splits a tool content map into its cleaned text content
// (reserved key removed) and any attached image results. The returned cleaned
// map is safe to json.Marshal and to persist on events.
func ExtractImageParts(content map[string]any) (map[string]any, []ImageResult) {
	results := []ImageResult{}
	if raw, ok := content[imagePartsKey]; ok {
		if r, ok := raw.([]ImageResult); ok {
			results = r
		}
	}
	cleaned := make(map[string]any, len(content))
	for k, v := range content {
		if k == imagePartsKey {
			continue
		}
		cleaned[k] = v
	}
	return cleaned, results
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/tool/ -v`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/tool/parts.go internal/tool/parts_test.go
git commit -m "feat(tool): add WithImageParts/ExtractImageParts multimodal tool-result channel"
```

---

### 任务 3：workspace 路径安全（safe.go，纯函数）

**文件：**
- 创建：`internal/workspace/safe.go`
- 测试：`internal/workspace/safe_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/workspace/safe_test.go`：

```go
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

	bad := []string{"", ".", "/abs/path", "../x", "..", "a/../../b", "C:/x", "c:\\x", "uploads/../../../etc"}
	for _, in := range bad {
		if _, err := safeRelPath(in); err == nil {
			t.Fatalf("safeRelPath(%q) should error", in)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"report.pdf":          "report.pdf",
		"../../etc/passwd":    "passwd",
		"a/b\\c.txt":          "c.txt",
		"my report (1).md":    "my report (1).md",
		"bad\x00name.txt":     "badname.txt",
		"..":                  "file",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Fatalf("sanitizeName(%q)=%q want %q", in, got, want)
		}
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/workspace/ -run 'TestSafeRelPath|TestSanitizeName' -v`
预期：FAIL（包不存在 / undefined）。

- [ ] **步骤 3：实现 safe.go**

创建 `internal/workspace/safe.go`：

```go
// Package workspace provides a per-conversation persistent file workspace
// backed by a blob.Store. Files live under the "workspaces/<convID>/" prefix;
// agents interact through relative logical paths via built-in tools.
package workspace

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

// safeRelPath validates and normalizes a caller-supplied logical path. It
// returns a clean, slash-separated relative path confined to the workspace
// root: absolute paths, drive letters, empty paths, and any ".." traversal
// after cleaning are rejected.
func safeRelPath(p string) (string, error) {
	p = filepath.ToSlash(p)
	p = path.Clean(p)
	if p == "" || p == "." {
		return "", fmt.Errorf("path must not be empty")
	}
	if path.IsAbs(p) || strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("absolute paths are not allowed: %s", p)
	}
	// Windows drive letter (e.g. "C:/x").
	if len(p) >= 2 && p[1] == ':' {
		return "", fmt.Errorf("drive-letter paths are not allowed: %s", p)
	}
	if p == ".." || strings.HasPrefix(p, "../") {
		return "", fmt.Errorf("path traversal is not allowed: %s", p)
	}
	return p, nil
}

// sanitizeName reduces an upload filename to a safe base name (no directory
// components, no control characters, no traversal). It never returns empty.
func sanitizeName(name string) string {
	name = filepath.Base(filepath.ToSlash(name))
	var b strings.Builder
	for _, r := range name {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "" || out == "." || out == ".." {
		return "file"
	}
	return out
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/workspace/ -v`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/workspace/safe.go internal/workspace/safe_test.go
git commit -m "feat(workspace): add safe relative-path and filename sanitization"
```

---

### 任务 4：workspace 服务（Service：blob 封装、key 布局、上限、隔离）

**文件：**
- 创建：`internal/workspace/workspace.go`
- 测试：`internal/workspace/service_test.go`

服务对外用**逻辑相对路径**（如 `uploads/a.txt`），内部拼 `workspaces/<conv>/<rel>`。定义一个 api/引擎都依赖的最小接口，避免 `api` 包直接依赖具体类型（放 `workspace` 包导出即可，api 直接 import）。

- [ ] **步骤 1：编写失败的测试**

创建 `internal/workspace/service_test.go`：

```go
package workspace_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/workspace"
)

func newSvc(t *testing.T) *workspace.Service {
	t.Helper()
	st, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return workspace.New(st)
}

func TestWriteReadListDeleteRoundTrip(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if err := svc.WriteFile(ctx, "c1", "notes/summary.md", "# hi"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ReadFile(ctx, "c1", "notes/summary.md")
	if err != nil || got != "# hi" {
		t.Fatalf("read=%q err=%v", got, err)
	}
	entries, err := svc.ListFiles(ctx, "c1", "")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, e := range entries {
		found[e.Name] = e.Type
	}
	if found["notes"] != "dir" {
		t.Fatalf("want notes dir, got %v", entries)
	}
	sub, _ := svc.ListFiles(ctx, "c1", "notes")
	if len(sub) != 1 || sub[0].Name != "summary.md" || sub[0].Type != "file" {
		t.Fatalf("want single file under notes, got %v", sub)
	}
	if err := svc.DeleteFile(ctx, "c1", "notes/summary.md"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteFile(ctx, "c1", "notes/missing.md"); err != nil {
		t.Fatalf("delete missing must be idempotent: %v", err)
	}
}

func TestConversationIsolation(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	_ = svc.WriteFile(ctx, "c1", "secret.txt", "topsecret")
	if _, err := svc.ReadFile(ctx, "c2", "secret.txt"); err == nil {
		t.Fatal("c2 must not read c1's file")
	}
	entries, _ := svc.ListFiles(ctx, "c2", "")
	if len(entries) != 0 {
		t.Fatalf("c2 workspace must be empty, got %v", entries)
	}
}

func TestPathTraversalRejected(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if err := svc.WriteFile(ctx, "c1", "../escape.txt", "x"); err == nil {
		t.Fatal("traversal must be rejected")
	}
	if _, err := svc.ReadFile(ctx, "c1", "/etc/passwd"); err == nil {
		t.Fatal("absolute path must be rejected")
	}
}

func TestEmptyConversationRejected(t *testing.T) {
	svc := newSvc(t)
	if err := svc.WriteFile(context.Background(), "", "a.txt", "x"); err == nil {
		t.Fatal("empty convID must error")
	}
}

func TestWriteTooLarge(t *testing.T) {
	svc := newSvc(t)
	big := strings.Repeat("x", workspace.MaxWriteBytes+1)
	if err := svc.WriteFile(context.Background(), "c1", "big.txt", big); err == nil {
		t.Fatal("oversize write must error")
	}
}

func TestReadTruncates(t *testing.T) {
	svc := newSvc(t)
	// MaxReadBytes (64 KiB) < this length < MaxWriteBytes (256 KiB), so the
	// write is accepted but the read must be truncated at the read cap.
	long := strings.Repeat("y", workspace.MaxReadBytes+100)
	if err := svc.WriteFile(context.Background(), "c1", "long.txt", long); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ReadFile(context.Background(), "c1", "long.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, workspace.TruncatedMarker) {
		t.Fatalf("read must be truncated with marker, len=%d", len(got))
	}
}

func TestSaveUploadReadable(t *testing.T) {
	svc := newSvc(t)
	logical, err := svc.SaveUpload(context.Background(), "c1", "report.pdf", "body text")
	if err != nil {
		t.Fatal(err)
	}
	if logical != "uploads/report.pdf" {
		t.Fatalf("logical path=%q", logical)
	}
	got, err := svc.ReadFile(context.Background(), "c1", logical)
	if err != nil || got != "body text" {
		t.Fatalf("upload not readable at uploads/ path: %q err=%v", got, err)
	}
}

func TestReadImageReturnsPart(t *testing.T) {
	svc := newSvc(t)
	// PNG signature bytes (http.DetectContentType -> image/png).
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}
	if _, err := svc.SaveUploadBytes(context.Background(), "c1", "shot.png", png, "image/png"); err != nil {
		t.Fatal(err)
	}
	part, ctype, err := svc.ReadImage(context.Background(), "c1", "uploads/shot.png")
	if err != nil {
		t.Fatal(err)
	}
	if part.Type != "image" || len(part.ImageBytes) == 0 {
		t.Fatalf("bad part %+v", part)
	}
	if !strings.HasPrefix(ctype, "image/") {
		t.Fatalf("content type %q not image", ctype)
	}
}

func TestReadImageRejectsText(t *testing.T) {
	svc := newSvc(t)
	_, _ = svc.SaveUpload(context.Background(), "c1", "a.txt", "not an image")
	if _, _, err := svc.ReadImage(context.Background(), "c1", "uploads/a.txt"); err == nil {
		t.Fatal("reading text as image must error")
	}
}

func TestResolveImagePart(t *testing.T) {
	svc := newSvc(t)
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	_, _ = svc.SaveUploadBytes(context.Background(), "c1", "x.png", png, "image/png")
	part, ok := svc.ResolveImagePart(context.Background(), "c1", "uploads/x.png")
	if !ok || part.Type != "image" {
		t.Fatalf("resolve failed: %+v ok=%v", part, ok)
	}
	if _, ok := svc.ResolveImagePart(context.Background(), "c1", "uploads/missing.png"); ok {
		t.Fatal("missing image must resolve to ok=false")
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/workspace/ -run 'TestWrite|TestConversation|TestPath|TestEmpty|TestSave|TestReadImage|TestResolve' -v`
预期：FAIL（`workspace.New` 等 undefined）。

- [ ] **步骤 3：实现 workspace.go**

创建 `internal/workspace/workspace.go`：

```go
package workspace

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/blob"
	"github.com/rebornace/baize/internal/llm"
)

const (
	rootPrefix   = "workspaces"
	uploadsDir   = "uploads"
	MaxWriteBytes = 256 << 10 // 256 KiB
	MaxReadBytes  = 64 << 10  // 64 KiB returned to the model
	MaxImageBytes = 10 << 20  // 10 MiB images
)

// TruncatedMarker is appended to read_file content cut at MaxReadBytes.
const TruncatedMarker = "…[truncated]"

// ErrNotFound is returned when a workspace file does not exist.
var ErrNotFound = errors.New("workspace: file not found")

// Entry is one logical directory entry returned by ListFiles.
type Entry struct {
	Name string `json:"name"`
	Type string `json:"type"` // "file" | "dir"
	Size int64  `json:"size,omitempty"`
}

// Service manages per-conversation workspaces on top of a blob.Store.
type Service struct {
	blobs    blob.Store
	visionOK func() bool
}

// Option configures a Service.
type Option func(*Service)

// WithVision supplies a vision-capability probe (returns true if the active
// model accepts image parts). When nil, read_image assumes vision is off.
func WithVision(f func() bool) Option {
	return func(s *Service) { s.visionOK = f }
}

// New builds a Service over the given blob store.
func New(blobs blob.Store, opts ...Option) *Service {
	s := &Service{blobs: blobs, visionOK: func() bool { return false }}
	for _, o := range opts {
		o(s)
	}
	return s
}

// VisionEnabled reports whether the active model supports images.
func (s *Service) VisionEnabled() bool { return s.visionOK != nil && s.visionOK() }

func requireConv(convID string) error {
	if strings.TrimSpace(convID) == "" {
		return errors.New("workspace requires a conversation context")
	}
	return nil
}

// key maps a logical relative path to a blob key for the conversation.
func (s *Service) key(convID, rel string) string {
	return path.Join(rootPrefix, convID, rel)
}

// rel maps a blob key back to a logical path within the conversation.
func (s *Service) rel(convID, key string) string {
	prefix := path.Join(rootPrefix, convID) + "/"
	return strings.TrimPrefix(key, prefix)
}

// SaveUpload persists an extracted text upload under uploads/ and returns its
// logical path (e.g. "uploads/report.pdf").
func (s *Service) SaveUpload(ctx context.Context, convID, filename, text string) (string, error) {
	if err := requireConv(convID); err != nil {
		return "", err
	}
	logical := path.Join(uploadsDir, sanitizeName(filename))
	if err := s.blobs.Put(ctx, s.key(convID, logical), []byte(text), "text/plain; charset=utf-8"); err != nil {
		return "", err
	}
	return logical, nil
}

// SaveUploadBytes persists an image upload (raw bytes) under uploads/ and
// returns its logical path.
func (s *Service) SaveUploadBytes(ctx context.Context, convID, filename string, data []byte, mime string) (string, error) {
	if err := requireConv(convID); err != nil {
		return "", err
	}
	if mime == "" {
		mime = http.DetectContentType(data)
	}
	logical := path.Join(uploadsDir, sanitizeName(filename))
	if err := s.blobs.Put(ctx, s.key(convID, logical), data, mime); err != nil {
		return "", err
	}
	return logical, nil
}

// WriteFile stores content at a logical path (whole-file overwrite).
func (s *Service) WriteFile(ctx context.Context, convID, relPath, content string) error {
	if err := requireConv(convID); err != nil {
		return err
	}
	rel, err := safeRelPath(relPath)
	if err != nil {
		return err
	}
	if len(content) > MaxWriteBytes {
		return fmt.Errorf("file too large: %d bytes > %d limit", len(content), MaxWriteBytes)
	}
	return s.blobs.Put(ctx, s.key(convID, rel), []byte(content), "text/plain; charset=utf-8")
}

// ReadFile returns UTF-8 text content, truncated at MaxReadBytes.
func (s *Service) ReadFile(ctx context.Context, convID, relPath string) (string, error) {
	if err := requireConv(convID); err != nil {
		return "", err
	}
	rel, err := safeRelPath(relPath)
	if err != nil {
		return "", err
	}
	b, err := s.blobs.Get(ctx, s.key(convID, rel))
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return "", fmt.Errorf("%s: %w", rel, ErrNotFound)
		}
		return "", err
	}
	if ct := http.DetectContentType(b); strings.HasPrefix(ct, "image/") {
		return "", fmt.Errorf("%s is a binary image; use read_image to view it", rel)
	}
	if len(b) > MaxReadBytes {
		// Cut on the byte budget, then back up to a valid rune boundary so we
		// never emit a partial multi-byte tail.
		cut := MaxReadBytes
		for cut > 0 && !utf8.RuneStart(b[cut]) {
			cut--
		}
		return string(b[:cut]) + TruncatedMarker, nil
	}
	return string(b), nil
}

// DeleteFile removes a file; deleting a missing file is a no-op success.
func (s *Service) DeleteFile(ctx context.Context, convID, relPath string) error {
	if err := requireConv(convID); err != nil {
		return err
	}
	rel, err := safeRelPath(relPath)
	if err != nil {
		return err
	}
	return s.blobs.Delete(ctx, s.key(convID, rel))
}

// ListFiles aggregates the flat blob key space into one directory level under
// dir (empty = workspace root). Entries are files or immediate subdirectories.
func (s *Service) ListFiles(ctx context.Context, convID, dir string) ([]Entry, error) {
	if err := requireConv(convID); err != nil {
		return nil, err
	}
	cleanDir := ""
	if strings.TrimSpace(dir) != "" {
		d, err := safeRelPath(dir)
		if err != nil {
			return nil, err
		}
		cleanDir = d
	}
	listPrefix := s.key(convID, cleanDir)
	if cleanDir != "" {
		listPrefix += "/"
	} else {
		listPrefix = path.Join(rootPrefix, convID) + "/"
	}
	objs, err := s.blobs.List(ctx, listPrefix)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []Entry
	for _, o := range objs {
		logical := strings.TrimPrefix(o.Key, listPrefix)
		if logical == "" {
			continue
		}
		first := logical
		isDir := false
		if idx := strings.IndexByte(logical, '/'); idx >= 0 {
			first = logical[:idx]
			isDir = true
		}
		if first == "" || seen[first] {
			continue
		}
		seen[first] = true
		if isDir {
			out = append(out, Entry{Name: first, Type: "dir"})
		} else {
			out = append(out, Entry{Name: first, Type: "file", Size: o.Size})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type == "dir" // dirs first
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// ReadImage returns an image content part and detected MIME for a logical path.
func (s *Service) ReadImage(ctx context.Context, convID, relPath string) (llm.ContentPart, string, error) {
	if err := requireConv(convID); err != nil {
		return llm.ContentPart{}, "", err
	}
	if !s.VisionEnabled() {
		return llm.ContentPart{}, "", errors.New("current model does not support vision; switch to a vision-capable model to view images")
	}
	rel, err := safeRelPath(relPath)
	if err != nil {
		return llm.ContentPart{}, "", err
	}
	b, err := s.blobs.Get(ctx, s.key(convID, rel))
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return llm.ContentPart{}, "", fmt.Errorf("%s: %w", rel, ErrNotFound)
		}
		return llm.ContentPart{}, "", err
	}
	if len(b) > MaxImageBytes {
		return llm.ContentPart{}, "", fmt.Errorf("image too large: %d bytes > %d limit", len(b), MaxImageBytes)
	}
	ct := http.DetectContentType(b)
	if !strings.HasPrefix(ct, "image/") {
		return llm.ContentPart{}, "", fmt.Errorf("%s is not an image (detected %s); use read_file for text", rel, ct)
	}
	return llm.ContentPart{Type: "image", ImageMIME: ct, ImageBytes: b}, ct, nil
}

// ResolveImagePart rebuilds an image part from a persisted image_refs pointer
// (used by the engine on cold resume). ok=false (no error) when missing.
func (s *Service) ResolveImagePart(ctx context.Context, convID, relPath string) (llm.ContentPart, bool) {
	part, _, err := s.ReadImage(ctx, convID, relPath)
	if err != nil {
		return llm.ContentPart{}, false
	}
	return part, true
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/workspace/ -v`
预期：PASS（safe_test.go + service_test.go 全绿）。

- [ ] **步骤 5：Commit**

```bash
git add internal/workspace/
git commit -m "feat(workspace): add per-conversation file Service over blob.Store"
```

---

### 任务 5：workspace 内置工具（5 个 ToolSpec + Invoker）

**文件：**
- 创建：`internal/workspace/tools.go`
- 测试：`internal/workspace/tools_test.go`

工具从 ctx 取 `identity.ConversationIDFrom`；参数缺失/路径非法/服务报错都返回 `isError=true` + `{"error": ...}`（不 panic、不返回 Go err，与 analysis 工具风格一致）。`read_image` 额外经 `tool.WithImageParts` 附带图片。

- [ ] **步骤 1：编写失败的测试**

创建 `internal/workspace/tools_test.go`：

```go
package workspace_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/workspace"
)

func ctxWithConv(conv string) context.Context {
	return identity.WithConversationID(context.Background(), conv)
}

func TestListAndWriteReadTools(t *testing.T) {
	svc := newSvc(t)
	tools := workspace.Tools(svc)
	reg := tool.NewRegistry()
	for _, tm := range tools {
		reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
	}
	ctx := ctxWithConv("c1")

	// write_file
	c, isErr, err := reg.Invoke(ctx, "write_file", map[string]any{"path": "notes/a.md", "content": "# hi"})
	if err != nil || isErr {
		t.Fatalf("write_file err=%v isErr=%v c=%v", err, isErr, c)
	}
	// list_files at root -> notes dir
	c, isErr, _ = reg.Invoke(ctx, "list_files", map[string]any{})
	if isErr {
		t.Fatalf("list_files isErr: %v", c)
	}
	entries, _ := c["entries"].([]workspace.Entry)
	if len(entries) != 1 || entries[0].Name != "notes" || entries[0].Type != "dir" {
		t.Fatalf("want notes dir, got %v", c)
	}
	// read_file
	c, isErr, _ = reg.Invoke(ctx, "read_file", map[string]any{"path": "notes/a.md"})
	if isErr {
		t.Fatalf("read_file isErr: %v", c)
	}
	if c["content"] != "# hi" {
		t.Fatalf("content=%v", c["content"])
	}
	// delete_file
	c, isErr, _ = reg.Invoke(ctx, "delete_file", map[string]any{"path": "notes/a.md"})
	if isErr {
		t.Fatalf("delete_file isErr: %v", c)
	}
}

func TestToolsRequireConversation(t *testing.T) {
	svc := newSvc(t)
	reg := tool.NewRegistry()
	for _, tm := range workspace.Tools(svc) {
		reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
	}
	_, isErr, err := reg.Invoke(context.Background(), "list_files", map[string]any{})
	if err != nil || !isErr {
		t.Fatalf("want isError without conversation, got isErr=%v err=%v", isErr, err)
	}
}

func TestReadImageToolAttachesPart(t *testing.T) {
	// vision-enabled service
	svc := memService(t, true)
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0, 0, 0, 0, 0}
	if _, err := svc.SaveUploadBytes(context.Background(), "c1", "s.png", png, "image/png"); err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	for _, tm := range workspace.Tools(svc) {
		reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
	}
	c, isErr, err := reg.Invoke(ctxWithConv("c1"), "read_image", map[string]any{"path": "uploads/s.png"})
	if err != nil || isErr {
		t.Fatalf("read_image err=%v isErr=%v c=%v", err, isErr, c)
	}
	_, results := tool.ExtractImageParts(c)
	if len(results) != 1 || results[0].Part.Type != "image" {
		t.Fatalf("want 1 image result attached, got %v", results)
	}
	if results[0].Path != "uploads/s.png" {
		t.Fatalf("image result path=%q", results[0].Path)
	}
}

func TestReadImageToolVisionGate(t *testing.T) {
	// default service: vision off
	svc := memService(t, false)
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}
	_, _ = svc.SaveUploadBytes(context.Background(), "c1", "s.png", png, "image/png")
	reg := tool.NewRegistry()
	for _, tm := range workspace.Tools(svc) {
		reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
	}
	_, isErr, err := reg.Invoke(ctxWithConv("c1"), "read_image", map[string]any{"path": "uploads/s.png"})
	if err != nil || !isErr {
		t.Fatalf("non-vision model must error, isErr=%v err=%v", isErr, err)
	}
}
```

tools_test.go 内自建 memory blob 并按 vision 开关构造 Service：

```go
func memService(t *testing.T, vision bool) *workspace.Service {
	t.Helper()
	b, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	opts := []workspace.Option{}
	if vision {
		opts = append(opts, workspace.WithVision(func() bool { return true }))
	}
	return workspace.New(b, opts...)
}
```

将 `TestReadImageToolAttachesPart` / `TestReadImageToolVisionGate` 中的服务构造改用 `memService(t, true)` / `memService(t, false)`，并在 vision 用例里对同一 svc 调 `SaveUploadBytes`（同一 blob）。`newSvc`（service_test.go）保持不变；tools_test.go 用自己的 `memService`。测试文件需 import `github.com/rebornace/baize/internal/blob` 与 `_ "github.com/rebornace/baize/internal/blob/memory"`。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/workspace/ -run 'TestListAndWriteReadTools|TestToolsRequire|TestReadImageTool' -v`
预期：FAIL（`workspace.Tools` undefined）。

- [ ] **步骤 3：实现 tools.go**

创建 `internal/workspace/tools.go`：

```go
package workspace

import (
	"context"
	"fmt"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/tool"
)

// ToolMeta pairs a tool spec with its invoker for registration.
type ToolMeta struct {
	Spec    llm.ToolSpec
	Invoker tool.Invoker
}

const workspaceToolHint = "This is a persistent, per-conversation file workspace. " +
	"Files you write and files the user uploads (under uploads/) persist across turns and runs " +
	"within this conversation. Paths are relative to the workspace root; absolute paths and '..' " +
	"are not allowed. Uploaded files are in uploads/."

func strArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok && s != ""
}

func fail(format string, a ...any) (map[string]any, bool, error) {
	return map[string]any{"error": fmt.Sprintf(format, a...)}, true, nil
}

// Tools returns the 5 built-in workspace tools for registration.
func Tools(svc *Service) []ToolMeta {
	listSpec := llm.ToolSpec{
		Name:        "list_files",
		Description: "List files and folders in your persistent workspace. Optional 'path' lists a subfolder (default: root). Returns entries with name and type (file/dir). Uploaded files are under uploads/. " + workspaceToolHint,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Optional folder path relative to workspace root"},
			},
		},
	}
	readSpec := llm.ToolSpec{
		Name:        "read_file",
		Description: "Read a UTF-8 text file from your workspace (e.g. an uploaded document under uploads/). Returns content (truncated if large). Use read_image for images. " + workspaceToolHint,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "File path relative to workspace root"},
			},
			"required": []string{"path"},
		},
	}
	writeSpec := llm.ToolSpec{
		Name:        "write_file",
		Description: "Write (create or overwrite) a UTF-8 text file in your workspace, e.g. to save notes or intermediate results for later turns. Parent folders are created automatically. Max 256 KiB. " + workspaceToolHint,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "File path relative to workspace root"},
				"content": map[string]any{"type": "string", "description": "Full file content"},
			},
			"required": []string{"path", "content"},
		},
	}
	deleteSpec := llm.ToolSpec{
		Name:        "delete_file",
		Description: "Delete a file from your workspace. Idempotent (deleting a missing file succeeds). " + workspaceToolHint,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "File path relative to workspace root"},
			},
			"required": []string{"path"},
		},
	}
	imageSpec := llm.ToolSpec{
		Name:        "read_image",
		Description: "View an image file from your workspace (e.g. an uploaded image under uploads/). The image is attached to this tool result for you to see. Requires a vision-capable model. " + workspaceToolHint,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Image path relative to workspace root"},
			},
			"required": []string{"path"},
		},
	}

	return []ToolMeta{
		{Spec: listSpec, Invoker: listInvoker(svc)},
		{Spec: readSpec, Invoker: readInvoker(svc)},
		{Spec: writeSpec, Invoker: writeInvoker(svc)},
		{Spec: deleteSpec, Invoker: deleteInvoker(svc)},
		{Spec: imageSpec, Invoker: readImageInvoker(svc)},
	}
}

func convFromCtx(ctx context.Context) (string, map[string]any, bool) {
	conv := identity.ConversationIDFrom(ctx)
	if conv == "" {
		m := map[string]any{"error": "workspace requires a conversation context"}
		return "", m, false
	}
	return conv, nil, true
}

func listInvoker(svc *Service) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		conv, m, ok := convFromCtx(ctx)
		if !ok {
			return m, true, nil
		}
		dir, _ := strArg(args, "path")
		entries, err := svc.ListFiles(ctx, conv, dir)
		if err != nil {
			return fail("%v", err)
		}
		return map[string]any{"path": dir, "entries": entries}, false, nil
	}
}

func readInvoker(svc *Service) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		conv, m, ok := convFromCtx(ctx)
		if !ok {
			return m, true, nil
		}
		p, ok := strArg(args, "path")
		if !ok {
			return fail("path is required")
		}
		content, err := svc.ReadFile(ctx, conv, p)
		if err != nil {
			return fail("%v", err)
		}
		return map[string]any{"path": p, "content": content}, false, nil
	}
}

func writeInvoker(svc *Service) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		conv, m, ok := convFromCtx(ctx)
		if !ok {
			return m, true, nil
		}
		p, ok := strArg(args, "path")
		if !ok {
			return fail("path is required")
		}
		content, _ := args["content"].(string)
		if err := svc.WriteFile(ctx, conv, p, content); err != nil {
			return fail("%v", err)
		}
		return map[string]any{"path": p, "bytes": len(content)}, false, nil
	}
}

func deleteInvoker(svc *Service) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		conv, m, ok := convFromCtx(ctx)
		if !ok {
			return m, true, nil
		}
		p, ok := strArg(args, "path")
		if !ok {
			return fail("path is required")
		}
		if err := svc.DeleteFile(ctx, conv, p); err != nil {
			return fail("%v", err)
		}
		return map[string]any{"path": p, "deleted": true}, false, nil
	}
}

func readImageInvoker(svc *Service) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		conv, m, ok := convFromCtx(ctx)
		if !ok {
			return m, true, nil
		}
		p, ok := strArg(args, "path")
		if !ok {
			return fail("path is required")
		}
		part, ctype, err := svc.ReadImage(ctx, conv, p)
		if err != nil {
			return fail("%v", err)
		}
		return tool.WithImageParts(map[string]any{
			"path":         p,
			"content_type": ctype,
			"bytes":        len(part.ImageBytes),
			"note":         "image attached to this tool result",
		}, tool.ImageResult{Path: p, Part: part}), false, nil
	}
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/workspace/ -v`
预期：PASS（safe / service / tools 全绿）。

- [ ] **步骤 5：Commit**

```bash
git add internal/workspace/
git commit -m "feat(workspace): add list_files/read_file/write_file/delete_file/read_image tools"
```

---

### 任务 6：引擎多模态工具结果通道（tool 消息图片 + 事件 image_refs + 冷恢复重取）

**文件：**
- 修改：`internal/run/engine.go`
- 测试：`internal/run/engine_workspace_test.go`（新建）

要点：
1. `Engine` 加字段 `ImagePartResolver func(convID, wsPath string) (llm.ContentPart, bool)`。
2. 新增辅助 `toolResultMessage(tcID string, content map[string]any) llm.Message`：`tool.ExtractImageParts(content)` 得 cleaned + results；无图 → `{Role:RoleTool, ToolCallID, Content: json(cleaned)}`；有图 → `{Role:RoleTool, ToolCallID, Parts: [text(json cleaned), image...] }`。
3. 新增辅助 `persistToolResult(runID, callID, name string, content map[string]any, isError bool)`：剥离图片，事件 content 用 cleaned，并附 `image_refs: [{workspace_path: r.Path}]`（仅当有图片）。
4. 三处接线：
   - `runLoop` 普通工具结果（当前 `engine.go` 约 520-531）：把 `content, _, tcErr := e.invokeTool(...)` 之后的 `raw,_ := json.Marshal(content)` + append 改为 `messages = append(messages, e.toolResultMessage(tc.ID, content))`；事件写入移入 `invokeTool` 统一（见下）。
   - `invokeTool` 内部事件（约 746-754）：改为调用 `e.persistToolResult(runID, callID, name, content, isError)`（剥离图片 + image_refs）。
   - HITL 冷恢复 `Resume`（约 354-376）：Invoke 后用 `e.persistToolResult(...)` 写事件；`eventsAfterInput` 重建图片。
   - skill 激活分支（约 499-517）不涉及图片，保持纯文本。
5. `eventsAfterInput` 的 `EventToolResult` 分支：读 `image_refs`；无 → 现状文本；有 → 经 `e.ImagePartResolver` 重建。因 `eventsAfterInput` 当前是包级函数，改为 `e` 的方法 `(e *Engine) eventsAfterInput(evs)` 以便访问 resolver；两处调用点（`buildResumeMessages` 内）相应改 `e.eventsAfterInput(evs)`。

- [ ] **步骤 1：编写失败的测试**

创建 `internal/run/engine_workspace_test.go`。测试基建复用既有模式：`store.NewMemory()`、`tool.NewRegistry()`、`st.UpsertAgent`、`st.CreateRun(store.CreateRunInput{...})`、`&Engine{Store: st, LLM: ..., Tools: reg, Gate: NewGate()}`（见 `engine_test.go`）。

```go
package run

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// visionStubLLM returns one read_image tool call then a final message, and
// captures every Chat call's messages.
type visionStubLLM struct {
	captured [][]llm.Message
	calls    int
}

func (s *visionStubLLM) SupportsVision() bool { return true }
func (s *visionStubLLM) Chat(_ context.Context, msgs []llm.Message, _ []llm.ToolSpec) (llm.Message, error) {
	s.captured = append(s.captured, msgs)
	s.calls++
	if s.calls == 1 {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
			ID: "call_1", Name: "read_image", Arguments: map[string]any{"path": "uploads/s.png"},
		}}}, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: "seen it"}, nil
}

func newWorkspaceEngine(st store.Store, llm_ llm.Provider) (*Engine, *tool.Registry) {
	reg := tool.NewRegistry()
	eng := &Engine{Store: st, LLM: llm_, Tools: reg, Gate: NewGate()}
	return eng, reg
}

func TestToolImagePartEncodedIntoToolMessage(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	llmStub := &visionStubLLM{}
	eng, reg := newWorkspaceEngine(st, llmStub)

	img := llm.ContentPart{Type: "image", ImageMIME: "image/png", ImageBytes: []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A}}
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "read_image"},
		func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
			return tool.WithImageParts(
				map[string]any{"path": "uploads/s.png", "note": "image attached"},
				tool.ImageResult{Path: "uploads/s.png", Part: img}), false, nil
		}, false)

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", ConversationID: "conv1", Input: "look"})
	if err != nil {
		t.Fatal(err)
	}
	ag := agent.Def{ID: "a", System: "sys"}
	if err := eng.Execute(context.Background(), r.ID, ag, "look"); err != nil {
		t.Fatal(err)
	}

	// 2nd Chat call: the tool message must carry an image part.
	if len(llmStub.captured) < 2 {
		t.Fatalf("expected >=2 Chat calls, got %d", llmStub.calls)
	}
	var foundImage bool
	for _, m := range llmStub.captured[1] {
		if m.Role == llm.RoleTool {
			for _, p := range m.Parts {
				if p.Type == "image" {
					foundImage = true
				}
			}
		}
	}
	if !foundImage {
		t.Fatalf("tool message did not carry an image part: %+v", llmStub.captured[1])
	}

	// Persisted ToolResult event: no reserved key leak; image_refs present.
	evs, _ := st.ListEvents(r.ID)
	sawRefs := false
	for _, ev := range evs {
		if ev.Type != EventToolResult {
			continue
		}
		content, _ := ev.Data["content"].(map[string]any)
		if _, leak := content["__baize_image_parts__"]; leak {
			t.Fatalf("image parts leaked into event content: %v", content)
		}
		refs, ok := ev.Data["image_refs"].([]map[string]any)
		if ok && len(refs) > 0 && refs[0]["workspace_path"] == "uploads/s.png" {
			sawRefs = true
		}
	}
	if !sawRefs {
		t.Fatalf("expected image_refs with workspace_path on tool.result event")
	}
}

func imageRefEvent() []store.Event {
	return []store.Event{{
		Type: EventToolResult,
		Data: map[string]any{
			"tool_call_id": "c1",
			"name":         "read_image",
			"content":      map[string]any{"path": "uploads/s.png"},
			"image_refs":   []map[string]any{{"workspace_path": "uploads/s.png"}},
		},
	}}
}

func TestColdResumeRebuildsImageFromResolver(t *testing.T) {
	eng, _ := newWorkspaceEngine(store.NewMemory(), &visionStubLLM{})
	eng.ImagePartResolver = func(_, _ string) (llm.ContentPart, bool) {
		return llm.ContentPart{Type: "image", ImageMIME: "image/png", ImageBytes: []byte("x")}, true
	}
	msgs := eng.eventsAfterInput(imageRefEvent(), "conv1")
	var gotImage bool
	for _, m := range msgs {
		if m.Role == llm.RoleTool {
			for _, p := range m.Parts {
				if p.Type == "image" {
					gotImage = true
				}
			}
		}
	}
	if !gotImage {
		t.Fatalf("cold resume did not rebuild image part: %+v", msgs)
	}
}

func TestColdResumeDegradesToTextWhenResolverFails(t *testing.T) {
	eng, _ := newWorkspaceEngine(store.NewMemory(), &visionStubLLM{})
	eng.ImagePartResolver = func(_, _ string) (llm.ContentPart, bool) { return llm.ContentPart{}, false }
	msgs := eng.eventsAfterInput(imageRefEvent(), "conv1")
	var toolMsg *llm.Message
	for i := range msgs {
		if msgs[i].Role == llm.RoleTool {
			toolMsg = &msgs[i]
		}
	}
	if toolMsg == nil {
		t.Fatal("no tool message")
	}
	if len(toolMsg.Parts) != 0 || !strings.Contains(toolMsg.Content, "read_image") {
		t.Fatalf("expected text degradation mentioning read_image, got %+v", toolMsg)
	}
}
```

注意：`eventsAfterInput` 改为方法后需要 conversationID 来调 resolver，签名为 `(e *Engine) eventsAfterInput(evs []store.Event, convID string) []llm.Message`；`buildResumeMessages` 已有 `conversationID` 参数，直接透传。无 resolver（nil）时等同于 resolver 失败 → 文本降级。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/run/ -run 'TestToolImagePart|TestColdResume' -v`
预期：FAIL（`eng.eventsAfterInput` 不是方法、`ImagePartResolver` 字段不存在；`tool.ImageResult` 已在任务 2 定义）。

- [ ] **步骤 3：引擎改动**

在 `internal/run/engine.go`：

(a) `Engine` 结构体加字段（放在 `OutboundExtras` 字段之后）：

```go
	// ImagePartResolver rebuilds image parts for persisted image_refs on cold
	// resume. nil (or ok=false) degrades that tool result to a text note.
	ImagePartResolver func(conversationID, workspacePath string) (llm.ContentPart, bool)
```

(b) 新增两个辅助方法（放在 `eventsAfterInput` 附近）：

```go
// toolResultMessage builds the LLM tool message for one tool invocation. When
// the invoker attached image results, the message is multimodal (a text part
// with the JSON content followed by image parts); otherwise it is plain text.
func (e *Engine) toolResultMessage(tcID string, content map[string]any) llm.Message {
	cleaned, results := tool.ExtractImageParts(content)
	raw, _ := json.Marshal(cleaned)
	if len(results) == 0 {
		return llm.Message{Role: llm.RoleTool, ToolCallID: tcID, Content: string(raw)}
	}
	parts := make([]llm.ContentPart, 0, len(results)+1)
	parts = append(parts, llm.ContentPart{Type: "text", Text: string(raw)})
	for _, r := range results {
		parts = append(parts, r.Part)
	}
	return llm.Message{Role: llm.RoleTool, ToolCallID: tcID, Parts: parts}
}

// persistToolResult appends the tool.result event. Image bytes are stripped
// (never persisted to events); lightweight image_refs pointers are recorded so
// a cold resume can rebuild the image via ImagePartResolver.
func (e *Engine) persistToolResult(runID, callID, name string, content map[string]any, isError bool) {
	cleaned, results := tool.ExtractImageParts(content)
	data := map[string]any{
		"tool_call_id": callID,
		"name":         name,
		"content":      identity.RedactSensitive(cleaned),
		"is_error":     isError,
	}
	if len(results) > 0 {
		refs := make([]map[string]any, 0, len(results))
		for _, r := range results {
			refs = append(refs, map[string]any{"workspace_path": r.Path})
		}
		data["image_refs"] = refs
	}
	_ = e.Store.AppendEvent(runID, store.Event{Type: EventToolResult, Data: data})
}

// imageRefsFromEvent extracts workspace paths from a persisted image_refs
// value, tolerating both in-memory ([]map[string]any) and JSON-round-tripped
// ([]any of map[string]any) shapes.
func imageRefsFromEvent(data map[string]any) []string {
	raw, ok := data["image_refs"]
	if !ok {
		return nil
	}
	var paths []string
	switch refs := raw.(type) {
	case []map[string]any:
		for _, ref := range refs {
			if p, ok := ref["workspace_path"].(string); ok && p != "" {
				paths = append(paths, p)
			}
		}
	case []any:
		for _, item := range refs {
			if m, ok := item.(map[string]any); ok {
				if p, ok := m["workspace_path"].(string); ok && p != "" {
					paths = append(paths, p)
				}
			}
		}
	}
	return paths
}

// toolResultFromEvent rebuilds a tool message from a persisted event,
// re-attaching image parts via the resolver when image_refs are present.
func (e *Engine) toolResultFromEvent(ev store.Event, convID string) llm.Message {
	tcID := asString(ev.Data["tool_call_id"])
	raw, _ := json.Marshal(ev.Data["content"])
	msg := llm.Message{Role: llm.RoleTool, ToolCallID: tcID, Content: string(raw)}
	refs := imageRefsFromEvent(ev.Data)
	var parts []llm.ContentPart
	for _, wsPath := range refs {
		if e.ImagePartResolver == nil {
			break
		}
		if part, ok := e.ImagePartResolver(convID, wsPath); ok {
			parts = append(parts, part)
		}
	}
	if len(parts) > 0 {
		msg.Parts = append([]llm.ContentPart{{Type: "text", Text: string(raw)}}, parts...)
		msg.Content = ""
	} else if len(refs) > 0 {
		msg.Content = string(raw) + "\n(image available at " + strings.Join(refs, ", ") + " — call read_image to view)"
	}
	return msg
}
```

(c) `runLoop` 普通工具结果分支（约 520-531），把：

```go
				content, _, tcErr := e.invokeTool(ctx, runID, tc.ID, tc.Name, tc.Arguments, false)
				if tcErr != nil && errors.Is(tcErr, ErrHITLRejected) {
					return tcErr
				}
				raw, _ := json.Marshal(content)
				messages = append(messages, llm.Message{
					Role:       llm.RoleTool,
					ToolCallID: tc.ID,
					Content:    string(raw),
				})
```

改为：

```go
				content, _, tcErr := e.invokeTool(ctx, runID, tc.ID, tc.Name, tc.Arguments, false)
				if tcErr != nil && errors.Is(tcErr, ErrHITLRejected) {
					return tcErr
				}
				messages = append(messages, e.toolResultMessage(tc.ID, content))
```

(d) `invokeTool` 内部事件写入（约 746-754），把整段 `e.Store.AppendEvent(runID, store.Event{Type: EventToolResult, Data: map[string]any{...}})` 改为：

```go
	e.persistToolResult(runID, callID, name, content, isError)
```

(e) HITL 冷恢复 `Resume`（约 366-376）里 Invoke 后的那段 `AppendEvent(EventToolResult, ...)` 同样改为：

```go
	e.persistToolResult(runID, toolCallID, payload.ToolName, content, isError)
```

(f) 把包级函数 `eventsAfterInput(evs []store.Event) []llm.Message` 改为方法 `(e *Engine) eventsAfterInput(evs []store.Event, convID string) []llm.Message`；其中 `EventToolResult` 分支由

```go
		case EventToolResult:
			flushPending()
			raw, _ := json.Marshal(ev.Data["content"])
			out = append(out, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: asString(ev.Data["tool_call_id"]),
				Content:    string(raw),
			})
```

改为：

```go
		case EventToolResult:
			flushPending()
			out = append(out, e.toolResultFromEvent(ev, convID))
```

(g) `buildResumeMessages` 内调用点改为传 convID：

```go
	messages = append(messages, e.eventsAfterInput(evs, conversationID)...)
```

（`eventsAfterInput` 仅此一个调用点；若编译报其他调用点，一并改为 `e.eventsAfterInput(evs, conversationID)`。）skill 激活分支（约 499-517）不涉及图片，保持原样。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/run/ -v`
预期：PASS（新测试 + 既有 engine/hitl/compact 等测试全绿）。

- [ ] **步骤 5：Commit**

```bash
git add internal/run/engine.go internal/run/engine_workspace_test.go
git commit -m "feat(run): multimodal tool-result channel (image parts, image_refs, cold-resume rebuild)"
```

---

### 任务 7：api 附件持久化 + 提示注入

**文件：**
- 修改：`internal/api/server.go`（`Server` 加 `Workspace` 字段 + 接口；`handlePostRun` 落附件、注入清单）
- 测试：`internal/api/server_workspace_test.go`（新建）

- [ ] **步骤 1：编写失败的测试**

创建 `internal/api/server_workspace_test.go`（`package api_test`，复用 `jsonBody`、`fakeRunner`，见 `server_test.go`）：

```go
package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/workspace"
)

type failingWorkspace struct{}

func (failingWorkspace) SaveUpload(context.Context, string, string, string) (string, error) {
	return "", errors.New("boom")
}
func (failingWorkspace) SaveUploadBytes(context.Context, string, string, []byte, string) (string, error) {
	return "", errors.New("boom")
}

func TestPostRunPersistsTextAttachmentToWorkspace(t *testing.T) {
	mem := store.NewMemory()
	mem.UpsertAgent(store.Agent{ID: "a", System: "s"})
	reg := tool.NewRegistry()
	srv := api.NewServer(mem, reg, &fakeRunner{store: mem})

	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ws := workspace.New(blobs)
	srv.Workspace = ws
	h := srv.Handler()

	body := map[string]any{
		"agent_id":        "a",
		"input":           "summarize this",
		"conversation_id": "conv1",
		"attachments": []map[string]any{{
			"filename":       "note.txt",
			"media_type":     "text/plain",
			"content_base64": base64.StdEncoding.EncodeToString([]byte("hello file contents")),
		}},
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/runs", jsonBody(t, body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	got, err := blobs.Get(context.Background(), "workspaces/conv1/uploads/note.txt")
	if err != nil {
		t.Fatalf("attachment not persisted: %v", err)
	}
	if string(got) != "hello file contents" {
		t.Fatalf("persisted content=%q", got)
	}
}

func TestPostRunPersistsImageAttachmentToWorkspace(t *testing.T) {
	mem := store.NewMemory()
	mem.UpsertAgent(store.Agent{ID: "a", System: "s"})
	reg := tool.NewRegistry()
	srv := api.NewServer(mem, reg, &fakeRunner{store: mem})
	srv.LLM = visionStubProvider{} // SupportsVision() == true

	blobs, _ := blob.Open(context.Background(), "memory", blob.Options{})
	srv.Workspace = workspace.New(blobs, workspace.WithVision(func() bool { return true }))
	h := srv.Handler()

	// 1x1-ish PNG signature bytes (attach re-encodes thumbnails; any valid PNG
	// header is enough to route to the image branch). Use a real tiny PNG.
	png := tinyPNG()
	body := map[string]any{
		"agent_id":        "a",
		"input":           "look",
		"conversation_id": "conv1",
		"attachments": []map[string]any{{
			"filename":       "shot.png",
			"media_type":     "image/png",
			"content_base64": base64.StdEncoding.EncodeToString(png),
		}},
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/runs", jsonBody(t, body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := blobs.Get(context.Background(), "workspaces/conv1/uploads/shot.png"); err != nil {
		t.Fatalf("image not persisted: %v", err)
	}
}

func TestPostRunWorkspaceFailureDoesNotBlock(t *testing.T) {
	mem := store.NewMemory()
	mem.UpsertAgent(store.Agent{ID: "a", System: "s"})
	reg := tool.NewRegistry()
	srv := api.NewServer(mem, reg, &fakeRunner{store: mem})
	srv.Workspace = failingWorkspace{}
	h := srv.Handler()

	body := map[string]any{
		"agent_id":        "a",
		"input":           "hi",
		"conversation_id": "conv1",
		"attachments": []map[string]any{{
			"filename":       "note.txt",
			"media_type":     "text/plain",
			"content_base64": base64.StdEncoding.EncodeToString([]byte("x")),
		}},
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/runs", jsonBody(t, body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("workspace failure must not block run: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["run_id"] == nil || resp["run_id"] == "" {
		t.Fatalf("run should still be created: %v", resp)
	}
}
```

辅助：`visionStubProvider` 若 api 测试包中已有支持视觉的 stub 则复用；否则在本文件加一个最小实现：

```go
type visionStubProvider struct{}

func (visionStubProvider) Chat(context.Context, []llm.Message, []llm.ToolSpec) (llm.Message, error) {
	return llm.Message{}, nil
}
func (visionStubProvider) SupportsVision() bool { return true }
```

（需 import `github.com/rebornace/baize/internal/llm`。）

`tinyPNG()` 返回一个合法的最小 PNG 字节切片（可用硬编码的 1x1 透明 PNG base64 解码）；放在本文件：

```go
func tinyPNG() []byte {
	const b64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	b, _ := base64.StdEncoding.DecodeString(b64)
	return b
}
```

注意：图片上传走 `attach.processImage` 会缩放重编码，`shot.png` 名保持不变；落盘 key 为 `workspaces/conv1/uploads/shot.png`。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/api/ -run 'TestPostRunPersists|TestPostRunWorkspaceFailure' -v`
预期：FAIL（`srv.Workspace` 字段不存在）。

- [ ] **步骤 3：Server 加 Workspace 字段与接口**

在 `internal/api/server.go` 的 `Server` 结构体中（`Artifacts` 字段附近）加入：

```go
	// Workspace optionally persists chat attachments to the per-conversation
	// file workspace. nil = attachments are not persisted (in-turn only).
	Workspace UploadSaver
```

并在 `type Server struct` 之前定义接口（放在文件靠近 `artifact.Store` 用法处即可）：

```go
// UploadSaver persists chat attachments into the per-conversation file
// workspace. SaveUpload stores extracted text; SaveUploadBytes stores image
// bytes. Both return the workspace-relative logical path. Failures are
// non-fatal: callers log and continue (the turn still proceeds inline).
type UploadSaver interface {
	SaveUpload(ctx context.Context, conversationID, filename, text string) (string, error)
	SaveUploadBytes(ctx context.Context, conversationID, filename string, data []byte, mime string) (string, error)
}
```

（`context` 已在 server.go import。）

- [ ] **步骤 4：handlePostRun 落附件 + 注入清单**

在 `handlePostRun` 中，定位到 `attach.Process(...)` 之后、构建 `llmText` 的那段（当前约 1423–1493 行）。在 vision 检查之后加入持久化调用，并收集成功保存的逻辑路径。

(a) 在 `textExts, imageExts, err := attach.Process(...)` 与 vision 检查之后，`llmText := cleanedInput` 之前，插入：

```go
	// Persist attachments to the per-conversation workspace (best-effort:
	// failures are logged and never block the turn). Saved logical paths are
	// listed so the model knows the files are available to read later.
	var savedWorkspaceFiles []string
	if s.Workspace != nil && conv != "" {
		for _, t := range textExts {
			if p, perr := s.Workspace.SaveUpload(r.Context(), conv, t.Filename, t.Text); perr != nil {
				log.Printf("workspace: save text upload %q: %v", t.Filename, perr)
			} else {
				savedWorkspaceFiles = append(savedWorkspaceFiles, p)
			}
		}
		for _, im := range imageExts {
			if p, perr := s.Workspace.SaveUploadBytes(r.Context(), conv, im.Filename, im.ImageBytes, im.ImageMIME); perr != nil {
				log.Printf("workspace: save image upload %q: %v", im.Filename, perr)
			} else {
				savedWorkspaceFiles = append(savedWorkspaceFiles, p)
			}
		}
	}
```

(b) 清单注入。当前文本附件会被拼进 `llmText`（内联）。在 `llmText` 构建完成后、`startRun` 之前，对**当轮用户输入**追加工作区清单。定位到 `displayText := cleanedInput` 之前，加入：

```go
	if len(savedWorkspaceFiles) > 0 {
		llmText = strings.TrimRight(llmText, "\n") +
			"\n\n[工作区] 以下文件已保存到本会话工作区，后续轮次可用 read_file（文本）或 read_image（图片）按需读取：" +
			strings.Join(savedWorkspaceFiles, ", ")
		if len(userParts) > 0 {
			// Rebuild the leading text part to include the workspace note.
			userParts[0] = llm.ContentPart{Type: "text", Text: llmText}
		}
	}
```

说明：当无附件时 `userParts` 为空、`llmText == cleanedInput`，此分支不触发，走原有纯文本路径（`startRun` 的 `Input` 用 `displayText`，不含清单；清单只进 LLM 提示）。有附件时 `userParts[0]` 已是文本 part（见现有代码先 append text part 再 append image parts），更新其 `Text` 即可。`displayText` 保持不变（气泡只显示文本 + 文件名，不暴露清单）。

> 若 `len(userParts)==0` 但有文本附件的情况不存在（现有逻辑有附件时必建 userParts），此分支安全。实现时以实际代码为准：确保清单文本进入发给 LLM 的内容（`userParts` 文本 part 或纯 input），而**不**写入持久化气泡 `displayText`。

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./internal/api/ -v`
预期：PASS（新测试 + 既有 api 测试全绿）。

- [ ] **步骤 6：Commit**

```bash
git add internal/api/server.go internal/api/server_workspace_test.go
git commit -m "feat(api): persist text/image attachments to conversation workspace + inject file list"
```

---

### 任务 8：bootstrap 装配（注册 5 工具、注入 api 与 engine）

**文件：**
- 修改：`internal/bootstrap/bootstrap.go`

装配发生在已有的 `if sqlBackend != nil { ... openBlobStore ... }` 块内（`blobStore`、`reg`、`srv`、`engine`、`provider` 均在作用域）。无独立 bootstrap 测试文件，验证靠 `go build ./...` 与既有测试 + 一个内存装配冒烟断言。

- [ ] **步骤 1：接线**

在 `internal/bootstrap/bootstrap.go` 中，定位 `if sqlBackend != nil {` 块（约 343–356 行），在 `srv.Artifacts = artStore` 之后加入 workspace 装配：

```go
		// Per-conversation file workspace reuses the same blob.Store
		// (artifacts use the "artifacts/" prefix; workspace uses
		// "workspaces/"). Register the 5 built-in file tools and wire both
		// the API (attachment persistence) and the engine (cold-resume image
		// rebuild).
		ws := workspace.New(blobStore, workspace.WithVision(func() bool {
			return provider != nil && provider.SupportsVision()
		}))
		for _, tm := range workspace.Tools(ws) {
			reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
		}
		srv.Workspace = ws
		engine.ImagePartResolver = func(convID, wsPath string) (llm.ContentPart, bool) {
			return ws.ResolveImagePart(context.Background(), convID, wsPath)
		}
```

并在文件顶部 import 块加入：

```go
	"github.com/rebornace/baize/internal/workspace"
```

（`llm`、`context` 应已在 bootstrap.go import；若 `llm` 未 import 则一并加入 `github.com/rebornace/baize/internal/llm`。）

- [ ] **步骤 2：编写冒烟测试（可选但推荐）**

创建 `internal/bootstrap/bootstrap_workspace_test.go`，用最小配置走 `Bootstrap` 成本较高（依赖 sqlite/目录）；更轻量的做法是直接断言「workspace 工具可注册到 registry 且名字正确」：

```go
package bootstrap

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/workspace"
)

func TestWorkspaceToolsRegistered(t *testing.T) {
	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	ws := workspace.New(blobs, workspace.WithVision(func() bool { return true }))
	for _, tm := range workspace.Tools(ws) {
		reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
	}
	want := map[string]bool{
		"list_files": false, "read_file": false, "write_file": false,
		"delete_file": false, "read_image": false,
	}
	for _, info := range reg.List() {
		if _, ok := want[info.Name]; ok {
			want[info.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("workspace tool %q not registered", name)
		}
	}
}
```

- [ ] **步骤 3：全量构建与测试**

运行：`go build ./... && go vet ./... && go test ./internal/... `
预期：构建通过、vet 无新增告警、所有包测试 PASS（blob / tool / workspace / run / api / bootstrap）。

- [ ] **步骤 4：手动冒烟（可选）**

以 file 驱动启动 demo，创建会话并发送一个文本附件 + 一句「总结」；随后新一轮问「列出工作区文件」并要求 `read_file`；再发一张图片（视觉模型）并在下一轮让 Agent `read_image`。确认：
- `<dataDir>/workspaces/<conv>/uploads/` 下出现文本与图片文件；
- Agent 能 list/read 文本、能 read_image 看图；
- 路径 `..` 被拒。

- [ ] **步骤 5：Commit**

```bash
git add internal/bootstrap/
git commit -m "feat(bootstrap): wire per-conversation workspace (tools, API saver, engine image resolver)"
```

---

## 自检结果（计划作者已核对）

- **规格覆盖：** §2.1 blob.List → 任务 1；§2.2/§2.3 key 布局与路径安全 → 任务 3、4；§2.4 文本工具 → 任务 5；§3.1 附件落工作区（文本+图片）→ 任务 7；§3.2 处理时机/提示注入 → 任务 7；§3.3 装配 → 任务 8；§3.4 错误与上限 → 任务 4/5（上限在 Service，错误消息在工具）；§4.1 图片持久化 → 任务 7；§4.2 多模态工具结果通道 → 任务 2（助手）+ 任务 6（引擎）；§4.3 事件 image_refs + 冷恢复重取/降级 → 任务 6；§4.4 read_image 工具与视觉门控 → 任务 4/5；§4.5 中间件队列无需改动 → 无代码任务（已在规格说明）；§3.5/§4.7 测试 → 分散在各任务。
- **类型一致性：** `tool.ImageResult{Path, Part}`、`tool.WithImageParts(content, ...ImageResult)`、`tool.ExtractImageParts(content) (map, []ImageResult)` 在任务 2 定义、任务 5/6 消费一致；`workspace.Service` 的 `SaveUpload/SaveUploadBytes` 均返回 `(string, error)`（逻辑路径），任务 4 定义、任务 7 消费一致；`Engine.ImagePartResolver func(convID, wsPath string) (llm.ContentPart, bool)` 与 `Service.ResolveImagePart(ctx, convID, path) (llm.ContentPart, bool)` 签名匹配（任务 8 用闭包适配 ctx）；`eventsAfterInput` 改为方法 `(e *Engine) eventsAfterInput(evs, convID)`。
- **占位符：** 无 TODO/待定；所有代码步骤均含完整代码。任务 6 的测试基建明确复用 `store.NewMemory()`/`tool.NewRegistry()`/`NewGate()`（与 `engine_test.go` 一致）。





