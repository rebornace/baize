# Agent 统计分析页 v0 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 内置工具 `create_analysis_page` 生成完整可交互统计分析 HTML 页（sections + 开放 ECharts option + 筛选/下钻/PDF；`format: html` 逃逸），存 artifact，`GET /v0/artifacts/{id}` 托管，聊天 iframe 预览；附带 `data-analytics` Skill 与 README。

**架构：** `internal/report` 将 sections 编译为自包含 HTML（embed ECharts + `runtime.js`）；`internal/artifact` 存 HTML 文件 + SQLite 元数据（`run_id` 鉴权）；`tool.Registry` 注册内置 invoker（从 `identity.RunIDFrom` 关联 run）；`runtime.js` 在浏览器端执行 filter、binding 聚合、drilldown、print PDF。`activate_skill` 仍由 Engine 特殊处理；`create_analysis_page` 走 Registry 常规 Invoke。

**技术栈：** Go 1.22+、`github.com/yuin/goldmark`（markdown section）、embed ECharts 5 min、`node --test`（runtime.js 单测）、React + Vite（聊天 UI）。

**规格：** `docs/superpowers/specs/2026-08-23-agent-analysis-page-v0-design.md`（已批准）

**全局约束：**
- 分支 `feat/analysis-page`，不在 `main` 直接改
- commit 中文 `type(scope): 说明`；PowerShell 不用 bash HEREDOC
- Go：`$env:GOPROXY='https://goproxy.cn,direct'`；`$env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH`
- UI 变更后：`cd web/chat && npm ci && npm run build`，提交 `internal/ui/dist/**`
- ECharts **embed 本地文件**，禁止 artifact 依赖外网 CDN
- 不实现 `create_interactive_report`、固定 bar/line/pie 模板

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/artifact/store.go` | `Store` 接口：`PutHTML(runID, html) (id, err)`、`Get(id)`、`RunID(id)` |
| `internal/artifact/file_store.go` | `{dir}/{id}.html` + SQLite `artifacts` 表 |
| `internal/artifact/store_test.go` | round-trip |
| `internal/report/types.go` | `PageRequest`、`Section`、`Filter`、`Dataset` 等 JSON 结构 |
| `internal/report/validate.go` | datasets 行宽、binding 引用、html 512KiB、禁外链 script |
| `internal/report/build.go` | sections → HTML document；html 逃逸包装 |
| `internal/report/markdown.go` | goldmark 渲染 markdown section |
| `internal/report/shell.html` | 页壳模板（筛选栏占位、PDF 按钮、section 容器） |
| `internal/report/assets/echarts.min.js` | ECharts 5（下载 vendoring） |
| `internal/report/runtime.js` | filter 状态、binding、drilldown、echarts init、print |
| `internal/report/embed.go` | `go:embed` assets + runtime + shell |
| `internal/report/build_test.go` | HTML 含 `__BAIZE_PAGE__`、CSP |
| `internal/report/validate_test.go` | 校验失败路径 |
| `internal/report/runtime_test.mjs` | Node 单测 binding/filter/drilldown |
| `internal/analysis/tool.go` | `CreateAnalysisPageToolName`、`ToolSpec()`、`Invoke(store, args)` |
| `internal/analysis/tool_test.go` | mock artifact store |
| `internal/bootstrap/bootstrap.go` | 注册 artifact store + `create_analysis_page` |
| `internal/api/server.go` | `Artifacts artifact.Store`；`GET /v0/artifacts/{id}` |
| `internal/api/server_artifacts_test.go` | GET 200/403/404 |
| `tests/integration/analysis_page_test.go` | mock LLM Run → tool → GET artifact |
| `examples/skills/data-analytics/SKILL.md` | Skill 包 |
| `web/chat/src/components/AnalysisPagePreview.tsx` | iframe + 新标签 |
| `web/chat/src/components/ToolCard.tsx` | 检测 `kind===analysis_page` 展示预览 |
| `web/chat/src/analysisPage.ts` | `parseAnalysisPageResult(content)` 纯函数 |
| `web/chat/src/analysisPage.test.ts` | 解析测试 |
| `web/chat/src/index.css` | `.analysis-page-preview` 样式 |
| `README.md` / `README.zh-CN.md` | 数据分析与报表章节 |
| `docs/superpowers/specs/2026-08-23-agent-analysis-page-v0-design.md` | 状态改为已批准 |

---

### 任务 0：功能分支

- [ ] **步骤 1：** `git checkout -b feat/analysis-page`

---

### 任务 1：Artifact 存储

**文件：**
- 创建：`internal/artifact/store.go`
- 创建：`internal/artifact/file_store.go`
- 创建：`internal/artifact/store_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
func TestFileStorePutGetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "b.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fs, err := artifact.NewFileStore(filepath.Join(dir, "artifacts"), st)
	if err != nil {
		t.Fatal(err)
	}
	id, err := fs.PutHTML("run_1", "<html><body>ok</body></html>")
	if err != nil {
		t.Fatal(err)
	}
	html, runID, err := fs.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if runID != "run_1" || !strings.Contains(html, "ok") {
		t.Fatalf("got run=%s html=%s", runID, html)
	}
}
```

- [ ] **步骤 2：** `go test ./internal/artifact -run TestFileStorePutGetRoundTrip -count=1` → FAIL

- [ ] **步骤 3：实现**

`store.go`：

```go
package artifact

type Store interface {
	PutHTML(runID string, html string) (id string, err error)
	Get(id string) (html string, runID string, err error)
}
```

`file_store.go`：
- `artifacts` 表：`id TEXT PRIMARY KEY, run_id TEXT, created_at INTEGER`
- `id` 格式 `art_` + 16 字节 hex
- HTML 写入 `dir/{id}.html`
- `Get` 读文件 + 查 `run_id`

在 `internal/store/sqlite.go` 的 migrate 路径增加 `CREATE TABLE IF NOT EXISTS artifacts (...)`（或 artifact 包内 `Open` 时 migrate，实现选一种并统一）。

- [ ] **步骤 4：** `go test ./internal/artifact -count=1` PASS

- [ ] **步骤 5：Commit** `feat(artifact): HTML 产物存储与 run 关联`

---

### 任务 2：report 类型与校验

**文件：**
- 创建：`internal/report/types.go`
- 创建：`internal/report/validate.go`
- 创建：`internal/report/validate_test.go`

- [ ] **步骤 1：失败测试** — `Validate` 对以下返回 error：
  - `echarts` section 无 `option` 且无 `binding`
  - `binding.dataset` 不存在
  - `datasets.tickets.rows` 列宽与 `columns` 不一致
  - `format: html` 且 `len(html) > 512*1024`
  - `html` 含 `<script src="https://`

- [ ] **步骤 2：** 运行 `go test ./internal/report -run Validate -count=1` → FAIL

- [ ] **步骤 3：实现** `types.go` 映射规格 §3.2 JSON；`validate.go` 纯函数 `func Validate(req *PageRequest) error`

- [ ] **步骤 4：** `go test ./internal/report -run Validate -count=1` PASS

- [ ] **步骤 5：Commit** `feat(report): 分析页请求校验`

---

### 任务 3：ECharts 静态资源 vendoring

**文件：**
- 创建：`internal/report/assets/echarts.min.js`

- [ ] **步骤 1：** 下载 ECharts 5.x `echarts.min.js` 到 `internal/report/assets/`（约 1MB；`curl -L` 或浏览器保存，版本记入 `assets/README` 一行）

- [ ] **步骤 2：Commit** `chore(report): vendoring echarts.min.js`

---

### 任务 4：页壳 HTML 与 sections 编译

**文件：**
- 创建：`internal/report/shell.html`
- 创建：`internal/report/markdown.go`
- 创建：`internal/report/build.go`
- 创建：`internal/report/embed.go`
- 修改：`go.mod`（`github.com/yuin/goldmark`）
- 测试：`internal/report/build_test.go`

- [ ] **步骤 1：失败测试**

```go
func TestBuildSectionsContainsPageJSON(t *testing.T) {
	req := &PageRequest{
		Title:  "Demo",
		Format: "sections",
		Datasets: map[string]Dataset{
			"t": {Columns: []string{"a"}, Rows: [][]string{{"1"}}},
		},
		Sections: []Section{{Type: "markdown", Content: "## Hi"}},
	}
	html, err := Build(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "__BAIZE_PAGE__") {
		t.Fatal("missing page json marker")
	}
	if !strings.Contains(html, "echarts") {
		t.Fatal("missing echarts embed")
	}
}
```

- [ ] **步骤 2：** FAIL

- [ ] **步骤 3：实现**

- `shell.html`：`<header>` 标题 + `#filter-bar` + `#export-pdf` 按钮 + `#sections` + `<script>` 注入 `window.__BAIZE_PAGE__` + embed echarts + runtime
- `Build(req)`：
  - `sections`：序列化 `datasets/filters/sections` 到 JSON，嵌入 shell
  - 静态 section 预渲染占位 div（`data-section-index`）
  - `html`：`WrapHTML(req.HTML)` — 若无 `<html` 则包 document；注入 PDF 按钮脚本（若未设 `export_pdf: false`）
- `markdown.go`：goldmark → HTML，无 raw `<script>`

- [ ] **步骤 4：** `go test ./internal/report -run TestBuildSections -count=1` PASS

- [ ] **步骤 5：Commit** `feat(report): sections 编译为自包含 HTML`

---

### 任务 5：runtime.js（筛选、binding、下钻、PDF）

**文件：**
- 创建：`internal/report/runtime.js`
- 创建：`internal/report/runtime_test.mjs`

- [ ] **步骤 1：编写 Node 测试**（`node --test internal/report/runtime_test.mjs`）

覆盖：
1. `applyFilters(rows, filterState, filters)` — select 过滤
2. `aggregateCount` / `groupByBarOption` — binding 简写生成 ECharts option
3. `drilldown set_filter` — 点击参数映射到 filter 值
4. `exportPDF` — 调用 `window.print`（mock）

- [ ] **步骤 2：** FAIL

- [ ] **步骤 3：实现 `runtime.js`**

核心导出（IIFE 挂 `window.BaizeReport`）：
- `init(page)` 读 `__BAIZE_PAGE__`
- 渲染 `filters` 到 `#filter-bar`（`select`、`date_range`：from/to input）
- 对每个 section index：`renderSection(i, filterState)`
  - `kpi` / `table` / `echarts+binding` 走 `compileBinding`
  - `echarts+option` 直接 `setOption`
- `drilldown`：`echartsInstance.on('click', ...)` 按 `action` 分支
- `#export-pdf` → `window.print()`
- `@media print` 样式内联在 shell CSS（隐藏 filter 控件）

- [ ] **步骤 4：** `node --test internal/report/runtime_test.mjs` PASS

- [ ] **步骤 5：Commit** `feat(report): 页内运行时筛选下钻与 PDF 打印`

---

### 任务 6：内置工具 `create_analysis_page`

**文件：**
- 创建：`internal/analysis/tool.go`
- 创建：`internal/analysis/tool_test.go`
- 修改：`internal/bootstrap/bootstrap.go`

- [ ] **步骤 1：失败测试** `Invoke` 返回 `artifact_url`、`kind: analysis_page`

```go
func TestCreateAnalysisPageInvoke(t *testing.T) {
	art := artifact.NewMemoryStore() // 测试用内存实现或 temp FileStore
	inv := analysis.Invoker(art)
	out, isErr, err := inv(identity.WithRunID(context.Background(), "run_x"), map[string]any{
		"title": "T", "format": "sections",
		"datasets": map[string]any{
			"d": map[string]any{"columns": []any{"c"}, "rows": []any{[]any{"1"}}},
		},
		"sections": []any{map[string]any{"type": "markdown", "content": "hi"}},
	})
	if err != nil || isErr {
		t.Fatalf("invoke: err=%v isErr=%v", err, isErr)
	}
	if out["kind"] != "analysis_page" || out["artifact_url"] == "" {
		t.Fatalf("out=%v", out)
	}
}
```

- [ ] **步骤 2：** FAIL

- [ ] **步骤 3：实现**

`tool.go`：
- `const ToolName = "create_analysis_page"`
- `ToolSpec()` — InputSchema 描述 `format`、`title`、`datasets`、`filters`、`sections`、`html`、`theme`
- `Invoker(art artifact.Store) tool.Invoker`：
  - `json` 解码 args → `report.PageRequest`
  - `report.Validate` → `is_error`
  - `report.Build` → `art.PutHTML(runID, html)`
  - 返回 `{artifact_id, artifact_url: "/v0/artifacts/"+id, kind, format, section_count}`

`bootstrap.go` `newAPIServer`：
- `artDir := filepath.Join(filepath.Dir(cfg.Store.SQLitePath), "artifacts")`
- `artStore, _ := artifact.NewFileStore(artDir, st)`
- `reg.RegisterSpec(analysis.ToolSpec(), analysis.Invoker(artStore))`
- `srv.Artifacts = artStore`（Server 新字段）

- [ ] **步骤 4：** `go test ./internal/analysis -count=1` PASS

- [ ] **步骤 5：Commit** `feat(analysis): create_analysis_page 内置工具`

---

### 任务 7：GET `/v0/artifacts/{id}`

**文件：**
- 修改：`internal/api/server.go`
- 创建：`internal/api/server_artifacts_test.go`

- [ ] **步骤 1：失败测试** — 有 token 且 run 存在 → 200 HTML；错误 id → 404

- [ ] **步骤 2：** FAIL

- [ ] **步骤 3：实现**

```go
func (s *Server) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	html, runID, err := s.Artifacts.Get(id)
	// 404 if missing
	// optional: s.Store.GetRun(runID) exists
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'")
	w.Write([]byte(html))
}
```

路由：`GET /v0/artifacts/{id}`；`MinRole` 与 `GET /v0/runs/{id}` 同级（`controlplane` 若需补路径规则则一并加）。

- [ ] **步骤 4：** `go test ./internal/api -run Artifact -count=1` PASS

- [ ] **步骤 5：Commit** `feat(api): GET artifacts HTML 托管`

---

### 任务 8：集成测试

**文件：**
- 创建：`tests/integration/analysis_page_test.go`

- [ ] **步骤 1：** mock LLM 第一步 `tool_calls: create_analysis_page`（最小 sections payload）→ Run succeeded → GET artifact 200 且 body 含 `BaizeReport` 或 `__BAIZE_PAGE__`

- [ ] **步骤 2：** `go test ./tests/integration -run AnalysisPage -count=1` PASS

- [ ] **步骤 3：Commit** `test(integration): 分析页工具端到端`

---

### 任务 9：Skill `data-analytics`

**文件：**
- 创建：`examples/skills/data-analytics/SKILL.md`

- [ ] **步骤 1：** 按规格 §7 frontmatter + 正文（`create_analysis_page`、`list_tickets`、`get_ticket`）

- [ ] **步骤 2：** `go test ./internal/skill -run Catalog -count=1` 确认 examples 路径可被测试加载（若已有 examples 扫描则通过）

- [ ] **步骤 3：Commit** `feat(skills): data-analytics 示例 Skill`

---

### 任务 10：聊天 UI iframe 预览

**文件：**
- 创建：`web/chat/src/analysisPage.ts`
- 创建：`web/chat/src/analysisPage.test.ts`
- 创建：`web/chat/src/components/AnalysisPagePreview.tsx`
- 修改：`web/chat/src/components/ToolCard.tsx`
- 修改：`web/chat/src/index.css`

- [ ] **步骤 1：失败测试** `analysisPage.test.ts`

```ts
import { parseAnalysisPageResult } from './analysisPage'

it('parses artifact_url', () => {
  const p = parseAnalysisPageResult({ kind: 'analysis_page', artifact_url: '/v0/artifacts/art_1' })
  expect(p?.artifactUrl).toBe('/v0/artifacts/art_1')
})
```

- [ ] **步骤 2：** `npm test -- analysisPage` FAIL

- [ ] **步骤 3：实现**

`parseAnalysisPageResult(content)`：从 tool result object 取 `kind`、`artifact_url`。

`AnalysisPagePreview.tsx`：
- `iframe` `sandbox="allow-scripts"` `src={url}` height 480
- `a` 新标签 `target="_blank" rel="noopener"`

`ToolCard.tsx`：展开 body 时若 `parseAnalysisPageResult(block.result)` 非空 → 渲染 `AnalysisPagePreview`（JSON pre 可保留在折叠「详情」或仅显示预览）

- [ ] **步骤 4：** `npm test` PASS

- [ ] **步骤 5：** `npm run build`；提交 `internal/ui/dist/**`

- [ ] **步骤 6：Commit** `feat(ui): 分析页 artifact iframe 预览`

---

### 任务 11：文档与规格状态

**文件：**
- 修改：`README.md`、`README.zh-CN.md`
- 修改：`docs/superpowers/specs/2026-08-23-agent-analysis-page-v0-design.md`（状态 → 已批准）

- [ ] **步骤 1：** 新增「数据分析与报表」：主路径 `create_analysis_page`、Token 策略、AntV MCP 可选静态图

- [ ] **步骤 2：** `go test ./...` 全绿

- [ ] **步骤 3：Commit** `docs: 数据分析页与 AntV 可选说明`

---

## 规格覆盖自检

| 规格 § | 任务 |
|--------|------|
| sections + binding + option | 2, 4, 5 |
| format: html 逃逸 | 2, 4 |
| 筛选 | 5 |
| 下钻 | 5 |
| 导出 PDF | 5 |
| artifact API + 鉴权 | 1, 7 |
| 聊天 iframe | 10 |
| data-analytics Skill | 9 |
| README | 11 |
| 不测 AntV 嵌入 | —（文档 only） |

---

**计划已完成。** 两种执行方式：

1. **子代理驱动（推荐）** — 每任务新子代理 + 审查  
2. **内联执行** — 本会话用 executing-plans 批量推进

选哪种方式？
