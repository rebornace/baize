# Tools 设置页可读目录 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 设置 → Tools 按 Connector / 路径前缀折叠并可搜索；人能改显示名与说明，换 spec 不冲掉人改文案。

**架构：** `store.Tool` 增加 `Title`（只给人看）与 `DescriptionCustom`。`MergeCatalog` 在 custom 时保留说明。启用行改说明走 `Registry.SetDescription`，不整工具重注册。GUI 纯函数分组/搜索，页面改成树 + 页顶添加抽屉 + 行内编辑。组头启停仍是多条现有 `PATCH enabled`。

**技术栈：** Go 1.22+、现有 httptest、React + Vite、vitest。不新加依赖。

**规格：** `docs/superpowers/specs/2026-08-17-tools-settings-ux-design.md`

**全局约束：**
- 不在 `main` 上改实现代码：先 `git checkout -b feat/tools-settings-ux`
- 不做批量目录 HTTP 接口，不用模型生成摘要
- 不改 `name`、方法、路径、`input_schema`、`require_approval`、会话身份、`auth.mode`、HITL 烘焙、控制面口令
- commit 中文 `type(scope): 说明`；PowerShell 不要 bash HEREDOC
- Go：`C:\Users\Administrator\sdk\go\bin`；`GOPROXY=https://goproxy.cn,direct`
- 每步 Go 测试：`$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH; go test <pkg> -count=1`
- 前端测试：在 `web/chat` 下 `npm test`

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/store/store.go` | `Tool.Title`、`Tool.DescriptionCustom` |
| `internal/store/sqlite.go` | 建表列、`ALTER` 迁移、读写 |
| `internal/store/store_test.go` / `sqlite_test.go` | 内存 CRUD 与旧库迁移 |
| `internal/connector/catalog.go` | 合并时保留 title / 人改说明 |
| `internal/connector/catalog_test.go` | 合并表驱动 |
| `internal/tool/registry.go` | `SetDescription` |
| `internal/tool/registry_test.go` | 原地更新描述；未知名报错 |
| `internal/api/server.go` | PATCH `title`/`description`；POST extra 的 title 与 custom |
| `internal/api/server_catalog_test.go` | PATCH 文案、换 spec 保留、空 body 400 |
| `internal/api/server_catalog_mutating_test.go` | 改说明不丢掉 mutating HITL |
| `web/chat/src/api.ts` | `title` / `description_custom`；PATCH 文案；POST `title` |
| `web/chat/src/toolCatalog.ts` | 路径前缀、搜索、树 |
| `web/chat/src/toolCatalog.test.ts` | 纯函数单测 |
| `web/chat/src/pages/ToolsSettings.tsx` | 树、搜索、抽屉、行内文案、组头逐条 PATCH |
| `web/chat/src/style.css` | 树 / 抽屉 / 搜索 |
| `README.md` / `README.zh-CN.md` / `docs/architecture-and-plugin-protocol.md` | 与实现一致 |
| `internal/ui/dist/**` | `npm run build` 产物 |

---

### 任务 1：Store 增加 title 与 description_custom

**文件：**
- 修改：`internal/store/store.go`
- 修改：`internal/store/sqlite.go`
- 修改：`internal/store/store_test.go`
- 修改：`internal/store/sqlite_test.go`

- [ ] **步骤 1：写失败测试**

在 `store_test.go` 的 `TestToolCatalogCRUD` 里，创建行时带上 `Title: "建工单"`、`DescriptionCustom: true`，`GetTool` 后断言这两字段仍在。

在 `sqlite_test.go` 追加：

```go
func TestSQLiteToolTitleAndCustomRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools-title.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	s.UpsertConnector(store.Connector{ID: "c1", Type: "openapi", BaseURL: "http://x"})
	s.UpsertTool(store.Tool{
		ConnectorID:        "c1",
		Name:               "create_ticket",
		Source:             store.ToolSourceSpec,
		Enabled:            true,
		Title:              "建工单",
		Description:        "人改的说明",
		DescriptionCustom:  true,
		Method:             "POST",
		Path:               "/tickets",
	})
	if c, ok := s.(io.Closer); ok {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}
	s2, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s2.(io.Closer); ok {
			_ = c.Close()
		}
	})
	got, err := s2.GetTool("create_ticket")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "建工单" || !got.DescriptionCustom || got.Description != "人改的说明" {
		t.Fatalf("got=%+v", got)
	}
}

func TestSQLiteToolColumnsMigrateFromOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old-tools.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE tools (
		name TEXT PRIMARY KEY, connector_id TEXT, source TEXT, enabled INTEGER,
		description TEXT, method TEXT, path TEXT, input_schema_json TEXT,
		require_login INTEGER, require_approval INTEGER, operation_id TEXT
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO tools (name, connector_id, source, enabled, description, method, path, require_login, require_approval, operation_id)
		VALUES ('create_ticket', 'c1', 'spec', 1, 'from spec', 'POST', '/tickets', 0, 0, 'create_ticket')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s.(io.Closer); ok {
			_ = c.Close()
		}
	})
	got, err := s.GetTool("create_ticket")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "" || got.DescriptionCustom || got.Description != "from spec" {
		t.Fatalf("migrated=%+v", got)
	}
}
```

- [ ] **步骤 2：跑测试确认失败**

```powershell
go test ./internal/store -count=1 -run "TestToolCatalogCRUD|TestSQLiteToolTitle|TestSQLiteToolColumnsMigrate"
```

预期：FAIL（未知字段或 SELECT 缺列）。

- [ ] **步骤 3：最少实现**

`store.Tool` 增加：

```go
Title             string `json:"title,omitempty"`
DescriptionCustom bool   `json:"description_custom"`
```

`description_custom` **不要** `omitempty`，这样 GET 在 false 时也会带回该字段。

`sqliteSchema` 的 `tools` 表增加 `title TEXT`、`description_custom INTEGER`。

`OpenSQLite` 在 `migrateRunsColumns` 之后调用 `migrateToolsColumns`：

```go
func migrateToolsColumns(db *sql.DB) error {
	alters := []string{
		`ALTER TABLE tools ADD COLUMN title TEXT`,
		`ALTER TABLE tools ADD COLUMN description_custom INTEGER`,
	}
	for _, q := range alters {
		_, err := db.Exec(q)
		if err == nil || isDuplicateColumnErr(err) {
			continue
		}
		return err
	}
	return nil
}
```

`loadConnectorsAndTools` 的 SELECT / Scan 增加 `title`、`description_custom`（整数 0/1）。`UpsertTool` 与 `ReplaceConnectorTools` 的 INSERT/UPSERT **两处都要**写入这两列，否则 Replace 会丢掉 overlay。

- [ ] **步骤 4：跑测试确认通过**

```powershell
go test ./internal/store -count=1
```

预期：PASS。

- [ ] **步骤 5：Commit**

```powershell
git add internal/store/store.go internal/store/sqlite.go internal/store/store_test.go internal/store/sqlite_test.go
git commit -m "feat(store): Tool 行增加显示名与人改说明标记"
```

---

### 任务 2：MergeCatalog 保留 title 与人改说明

**文件：**
- 修改：`internal/connector/catalog.go`
- 修改：`internal/connector/catalog_test.go`

- [ ] **步骤 1：写失败测试**

```go
func TestMergeCatalogPreservesTitleAndCustomDescription(t *testing.T) {
	existing := []store.Tool{
		{
			Name: "old", ConnectorID: "c", Source: store.ToolSourceSpec,
			Enabled: false, Title: "旧显示名", Description: "人改", DescriptionCustom: true,
		},
		{
			Name: "plain", ConnectorID: "c", Source: store.ToolSourceSpec,
			Enabled: true, Title: "仍保留", Description: "旧 spec", DescriptionCustom: false,
		},
		{
			Name: "extra1", ConnectorID: "c", Source: store.ToolSourceExtra,
			Enabled: true, Title: "手加", Description: "e", DescriptionCustom: true, Method: "GET", Path: "/e",
		},
	}
	discovered := []store.Tool{
		{Name: "old", ConnectorID: "c", Source: store.ToolSourceSpec, Method: "POST", Path: "/old", Description: "新 spec"},
		{Name: "plain", ConnectorID: "c", Source: store.ToolSourceSpec, Method: "GET", Path: "/p", Description: "新 spec plain"},
		{Name: "fresh", ConnectorID: "c", Source: store.ToolSourceSpec, Method: "GET", Path: "/n", Description: "fresh spec"},
	}
	out := MergeCatalog(MergeOpts{Existing: existing, Discovered: discovered})
	by := map[string]store.Tool{}
	for _, r := range out {
		by[r.Name] = r
	}
	if by["old"].Title != "旧显示名" || by["old"].Description != "人改" || !by["old"].DescriptionCustom || by["old"].Path != "/old" {
		t.Fatalf("old=%+v", by["old"])
	}
	if by["plain"].Title != "仍保留" || by["plain"].Description != "新 spec plain" || by["plain"].DescriptionCustom {
		t.Fatalf("plain=%+v", by["plain"])
	}
	if by["fresh"].Title != "" || by["fresh"].DescriptionCustom || by["fresh"].Description != "fresh spec" {
		t.Fatalf("fresh=%+v", by["fresh"])
	}
	if by["extra1"].Title != "手加" || !by["extra1"].DescriptionCustom {
		t.Fatalf("extra=%+v", by["extra1"])
	}
}
```

- [ ] **步骤 2：跑测试确认失败**

```powershell
go test ./internal/connector -count=1 -run TestMergeCatalogPreservesTitleAndCustomDescription
```

预期：FAIL，`old.Description` 被写成 `"新 spec"`。

- [ ] **步骤 3：最少实现**

在 `MergeCatalog` 对再次发现的 spec/plugin 行，现有逻辑是 `row = ex` 后覆盖 `Method`/`Path`/`Description`/`Source`/`OperationID`。改成：

- `Description`：仅当 `!ex.DescriptionCustom` 时用 `d.Description`
- `Title`、`DescriptionCustom`：不要赋成 discovered 的零值（`row = ex` 已保留）
- 其它覆盖规则不变

新发现行继续 `row := d`（title 空、custom false）。extra 仍整行原样放入。

- [ ] **步骤 4：跑测试确认通过**

```powershell
go test ./internal/connector -count=1 -run MergeCatalog
```

预期：PASS（含原有三条）。

- [ ] **步骤 5：Commit**

```powershell
git add internal/connector/catalog.go internal/connector/catalog_test.go
git commit -m "feat(connector): 换 spec 保留显示名与人改说明"
```

---

### 任务 3：Registry 原地更新描述 + PATCH/POST

**文件：**
- 修改：`internal/tool/registry.go`
- 修改：`internal/tool/registry_test.go`
- 修改：`internal/api/server.go`（`handlePatchTool`、`handlePostConnectorTool`）
- 修改：`internal/api/server_catalog_test.go`
- 修改：`internal/api/server_catalog_mutating_test.go`

- [ ] **步骤 1：写失败测试**

`registry_test.go`：

```go
func TestRegistrySetDescription(t *testing.T) {
	r := tool.NewRegistry()
	nop := func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{}, false, nil
	}
	r.RegisterMeta(tool.Meta{
		Spec: llm.ToolSpec{Name: "create_ticket", Description: "old"},
		ConnectorID: "ticket", Method: "POST", Path: "/tickets",
	}, nop, true)
	if err := r.SetDescription("create_ticket", "new"); err != nil {
		t.Fatal(err)
	}
	info, ok := r.Get("create_ticket")
	if !ok || info.Description != "new" || !info.RequireApproval {
		t.Fatalf("info=%+v ok=%v", info, ok)
	}
	if err := r.SetDescription("missing", "x"); err == nil {
		t.Fatal("expected unknown tool")
	}
}
```

`server_catalog_test.go` 追加（spec 里给 `get_ticket` 写 `description: spec-one`，第二次 PUT 改成 `spec-two`）：

```go
func TestCatalogPatchTitleAndDescription(t *testing.T) {
	dir := t.TempDir()
	spec1 := filepath.Join(dir, "a.yaml")
	spec2 := filepath.Join(dir, "b.yaml")
	body1 := `openapi: 3.0.3
info: {title: c, version: "0.1.0"}
paths:
  /tickets:
    post:
      operationId: create_ticket
      responses: {"201": {description: created}}
  /tickets/{id}:
    get:
      operationId: get_ticket
      description: spec-one
      parameters: [{name: id, in: path, required: true, schema: {type: string}}]
      responses: {"200": {description: ok}}
`
	body2 := strings.Replace(body1, "spec-one", "spec-two", 1)
	if err := os.WriteFile(spec1, []byte(body1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec2, []byte(body2), 0o644); err != nil {
		t.Fatal(err)
	}
	_, reg, _, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec1, "http://127.0.0.1:9")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/v0/tools/get_ticket",
		jsonBodyAPI(t, map[string]any{"title": "查工单", "description": "人改说明"})))
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH title+desc status=%d body=%s", rr.Code, rr.Body.String())
	}
	tools := getToolsList(t, h)
	gt, _ := findTool(tools, "get_ticket")
	if gt.Title != "查工单" || gt.Description != "人改说明" || !gt.DescriptionCustom {
		t.Fatalf("after patch=%+v", gt)
	}
	info, ok := reg.Get("get_ticket")
	if !ok || info.Description != "人改说明" {
		t.Fatalf("registry desc=%+v ok=%v", info, ok)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/v0/tools/get_ticket",
		jsonBodyAPI(t, map[string]any{"title": "只改标题"})))
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH title-only status=%d", rr.Code)
	}
	info, _ = reg.Get("get_ticket")
	if info.Description != "人改说明" {
		t.Fatalf("title-only must not change registry desc: %+v", info)
	}

	putCatalogConnector(t, h, "c1", spec2, "http://127.0.0.1:9")
	tools = getToolsList(t, h)
	gt, _ = findTool(tools, "get_ticket")
	if gt.Title != "只改标题" || gt.Description != "人改说明" || !gt.DescriptionCustom {
		t.Fatalf("after re-PUT=%+v", gt)
	}
	info, _ = reg.Get("get_ticket")
	if info.Description != "人改说明" {
		t.Fatalf("registry after re-PUT=%+v", info)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPatch, "/v0/tools/get_ticket", jsonBodyAPI(t, map[string]any{})))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty PATCH want 400 got %d", rr.Code)
	}
}

func TestCatalogPostExtraSetsTitleAndCustom(t *testing.T) {
	spec := writeCatalogSpec(t)
	_, _, _, h := catalogServer(t)
	putCatalogConnector(t, h, "c1", spec, "http://127.0.0.1:9")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/connectors/c1/tools",
		jsonBodyAPI(t, map[string]any{
			"name": "echo_extra", "method": "GET", "path": "/echo",
			"title": "回声", "description": "手加",
		})))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST extra status=%d body=%s", rr.Code, rr.Body.String())
	}
	tools := getToolsList(t, h)
	ex, ok := findTool(tools, "echo_extra")
	if !ok || ex.Title != "回声" || !ex.DescriptionCustom || ex.Source != store.ToolSourceExtra {
		t.Fatalf("extra=%+v ok=%v", ex, ok)
	}
}
```

`server_catalog_mutating_test.go` 在现有 mutating 夹具上，对 `create_ticket` PATCH `{"description":"人改"}`，然后 `reg.Get("create_ticket")` 仍 `RequireApproval==true` 且 Description 为「人改」。

- [ ] **步骤 2：跑测试确认失败**

```powershell
go test ./internal/tool -count=1 -run TestRegistrySetDescription
go test ./internal/api -count=1 -run "TestCatalogPatchTitleAndDescription|TestCatalogPostExtraSetsTitleAndCustom"
```

预期：FAIL（`SetDescription` 未定义；PATCH 400「至少 enabled 或 require_login」）。

- [ ] **步骤 3：最少实现**

`Registry.SetDescription` 与 `SetRequireLogin` 同模式：锁 map，改 `e.spec.Description`，未知名 `fmt.Errorf("unknown tool: %s")`。

`handlePatchTool` body：

```go
var body struct {
	Enabled      *bool   `json:"enabled"`
	RequireLogin *bool   `json:"require_login"`
	Title        *string `json:"title"`
	Description  *string `json:"description"`
}
```

四个全 nil → 400 `at least one of enabled, require_login, title, or description is required`。

赋值：`Title`/`Description` 非 nil 则写入；`Description` 非 nil 则 `DescriptionCustom = true`。然后 `UpsertTool`。

Registry 分流（保持今天 HITL 行为）：

- `!row.Enabled` → `Unregister`（与今天相同）
- `enabledChanged`（false→true）→ `registerOne`
- 否则：
  - `RequireLogin != nil` → 现有 `SetRequireLogin`，失败再 `registerOne`
  - `Description != nil` → `SetDescription`；若未知名则 `registerOne`
  - 只改 `Title`：不要碰 Registry

`handlePostConnectorTool`：body 加 `Title string`；构造 `store.Tool` 时 `Title: strings.TrimSpace(body.Title)`，`DescriptionCustom: true`。

- [ ] **步骤 4：跑测试确认通过**

```powershell
go test ./internal/tool -count=1
go test ./internal/api -count=1 -run "Catalog|PatchTool"
```

预期：PASS。现有 `TestCatalogPatchEnabled*` 与 mutating HITL 测试不得红。

- [ ] **步骤 5：Commit**

```powershell
git add internal/tool/registry.go internal/tool/registry_test.go internal/api/server.go internal/api/server_catalog_test.go internal/api/server_catalog_mutating_test.go
git commit -m "feat(api): PATCH 显示名与说明并原地更新 Registry 描述"
```

---

### 任务 4：路径前缀分组与搜索纯函数

**文件：**
- 修改：`web/chat/src/toolCatalog.ts`
- 修改：`web/chat/src/toolCatalog.test.ts`
- 修改：`web/chat/src/api.ts`

- [ ] **步骤 1：写失败测试**

`api.ts` 的 `ToolInfo` 增加可选 `title?: string`、`description_custom?: boolean`。`patchTool` 的 body 类型改为 `{ enabled?: boolean; require_login?: boolean; title?: string; description?: string }`。`createConnectorTool` body 增加可选 `title?: string`。

`toolCatalog.test.ts`（在现有 `canDeleteCatalogTool` 旁）：

```ts
import { pathPrefixGroup, toolMatchesQuery, groupToolsTree } from './toolCatalog'
import type { ToolInfo } from './api'

const sample = (over: Partial<ToolInfo> & Pick<ToolInfo, 'name' | 'connector_id'>): ToolInfo => ({
  method: 'GET',
  path: '/x',
  ...over,
})

describe('pathPrefixGroup', () => {
  it('skips api and version segments', () => {
    expect(pathPrefixGroup('/api/v1/tickets/{id}')).toBe('/tickets')
    expect(pathPrefixGroup('/tickets')).toBe('/tickets')
    expect(pathPrefixGroup('/v2/customers')).toBe('/customers')
    expect(pathPrefixGroup(undefined)).toBe('其他')
    expect(pathPrefixGroup('/api/v1')).toBe('其他')
  })
})

describe('toolMatchesQuery', () => {
  it('matches title case-insensitively', () => {
    const t = sample({ name: 'get_ticket', connector_id: 'c', title: '查工单', path: '/tickets/{id}', description: '按 id 取' })
    expect(toolMatchesQuery(t, '查工')).toBe(true)
    expect(toolMatchesQuery(t, 'TICKETS')).toBe(true)
    expect(toolMatchesQuery(t, 'zzz')).toBe(false)
    expect(toolMatchesQuery(t, '  ')).toBe(true)
  })
})

describe('groupToolsTree', () => {
  it('groups by connector then prefix, 其他 last', () => {
    const tools: ToolInfo[] = [
      sample({ name: 'a', connector_id: 'billing', path: '/invoices' }),
      sample({ name: 'b', connector_id: 'ticket', path: '/tickets' }),
      sample({ name: 'c', connector_id: 'ticket', path: '/comments' }),
      sample({ name: 'plug', connector_id: 'ticket', path: undefined, source: 'plugin' }),
    ]
    const tree = groupToolsTree(tools)
    expect(tree.map((g) => g.connectorId)).toEqual(['billing', 'ticket'])
    const ticket = tree[1]
    expect(ticket.prefixes.map((p) => p.prefix)).toEqual(['/comments', '/tickets', '其他'])
    expect(ticket.prefixes[2].tools.map((t) => t.name)).toEqual(['plug'])
  })
})
```

Connector 顺序 = 在传入数组中首次出现的顺序（`ListTools` 已按 name 排序，但跨 Connector 仍按遍历顺序收集）。同 Connector 内前缀字典序，「其他」最后。

- [ ] **步骤 2：跑测试确认失败**

```powershell
cd web/chat
npm test
```

预期：FAIL，导出函数未定义。

- [ ] **步骤 3：最少实现**

`toolCatalog.ts`：

```ts
const VERSION_SEG = /^v\d+(\.\d+)*$/i

export function pathPrefixGroup(path: string | undefined): string {
  if (path == null || path.trim() === '') return '其他'
  const segs = path.split('/').filter((s) => s.length > 0)
  let i = 0
  while (i < segs.length) {
    const seg = segs[i]
    if (seg.toLowerCase() === 'api' || VERSION_SEG.test(seg)) {
      i += 1
      continue
    }
    break
  }
  if (i >= segs.length) return '其他'
  return `/${segs[i]}`
}

export function toolMatchesQuery(t: ToolInfo, q: string): boolean {
  const needle = q.trim().toLowerCase()
  if (needle === '') return true
  const hay = [t.title, t.name, t.path, t.description, t.method]
  return hay.some((p) => (p ?? '').toLowerCase().includes(needle))
}

export interface ToolPrefixGroup {
  prefix: string
  tools: ToolInfo[]
}

export interface ConnectorGroup {
  connectorId: string
  prefixes: ToolPrefixGroup[]
}

export function groupToolsTree(tools: ToolInfo[]): ConnectorGroup[] {
  const connectorOrder: string[] = []
  const byConnector = new Map<string, ToolInfo[]>()
  for (const t of tools) {
    const id = t.connector_id || ''
    if (!byConnector.has(id)) {
      connectorOrder.push(id)
      byConnector.set(id, [])
    }
    byConnector.get(id)!.push(t)
  }
  return connectorOrder.map((connectorId) => {
    const rows = byConnector.get(connectorId) ?? []
    const prefixOrder: string[] = []
    const byPrefix = new Map<string, ToolInfo[]>()
    for (const t of rows) {
      const prefix = pathPrefixGroup(t.path)
      if (!byPrefix.has(prefix)) {
        prefixOrder.push(prefix)
        byPrefix.set(prefix, [])
      }
      byPrefix.get(prefix)!.push(t)
    }
    prefixOrder.sort((a, b) => {
      if (a === '其他') return 1
      if (b === '其他') return -1
      return a.localeCompare(b)
    })
    return {
      connectorId,
      prefixes: prefixOrder.map((prefix) => ({ prefix, tools: byPrefix.get(prefix) ?? [] })),
    }
  })
}
```

顶部 import `ToolInfo`（文件顶，禁止函数内 import）。

- [ ] **步骤 4：跑测试确认通过**

```powershell
cd web/chat
npm test
```

预期：PASS。

- [ ] **步骤 5：Commit**

```powershell
git add web/chat/src/api.ts web/chat/src/toolCatalog.ts web/chat/src/toolCatalog.test.ts
git commit -m "feat(ui): 工具目录按路径前缀分组并可搜索"
```

---

### 任务 5：Tools 设置页树、抽屉、行内文案

**文件：**
- 修改：`web/chat/src/pages/ToolsSettings.tsx`
- 修改：`web/chat/src/style.css`

- [ ] **步骤 1：按规格改页面（无点击测试；用纯函数 + 构建校验）**

结构要求（实现时对照，不要留底部常驻表单）：

1. 页顶：`<h1>Tools</h1>`、搜索 `<input>`、有 OpenAPI Connector 时「添加」按钮。
2. `query` state；`visible = tools.filter(t => toolMatchesQuery(t, query))`；`tree = groupToolsTree(visible)`。
3. Connector / 前缀展开：`Set<string>`。仅一个 Connector 且 `query` 为空时，默认展开该 Connector id。`query` 非空时展开所有含命中行的 Connector 与前缀。
4. Connector 组头：id、工具数、已启用数、「全部启用」「全部停用」。对该组 `visible` 工具名 `Promise.allSettled(names.map(n => patchTool(n, { enabled })))`。进行中禁用该组按钮。结束后：成功的行写入 `tools` state；失败则页顶 `已更新 k/n`，列出至多 5 个失败名+原因。
5. 前缀组头同样启停，范围是该前缀下当前可见工具。
6. 行：主标签 `t.title || t.name`；次行 `METHOD path`；截断 `description`；启用 / 需要登录 / extra 删除 / 「需审批」徽章。「编辑文案」展开 `title`、`description` 输入，保存 `patchTool(name, { title, description })`。失败不关编辑区，页顶报错，该行回到请求前。
7. 「添加」打开右侧抽屉（`settings-drawer`），字段与今天 extra 表单相同并加可选显示名；提交 `createConnectorTool`；成功关抽屉清空；失败留在抽屉。
8. 无 OpenAPI Connector：不渲染添加按钮（与今天隐藏表单相同）。
9. 加载失败 / 空目录 / 搜索无命中文案按规格 §8。

CSS（加在现有 `settings-*` 旁）：`.settings-tools { max-width: 52rem; }`、`.settings-toolbar`（搜索+添加横排）、`.settings-tree` / `.settings-group` / `.settings-group-head`、`.settings-tool-desc`（截断一行）、`.settings-drawer-backdrop` + `.settings-drawer`（右侧固定栏，宽约 22rem）。不要把 `.settings-section` 全局改宽，以免账号页被拉宽。

根元素：`<div className="settings-section settings-tools">`。

- [ ] **步骤 2：跑前端测试与构建**

```powershell
cd web/chat
npm test
npm run build
```

预期：vitest PASS；`tsc && vite build` 成功。把产物同步到 `internal/ui/dist`（与现有 Chat UI 构建流程相同）。

- [ ] **步骤 3：Commit**

```powershell
git add web/chat/src/pages/ToolsSettings.tsx web/chat/src/style.css internal/ui/dist
git commit -m "feat(ui): Tools 设置页树形目录、抽屉添加与行内文案"
```

---

### 任务 6：文档与回归

**文件：**
- 修改：`README.md`（Operator UI 那条 + `### Tool catalog`）
- 修改：`README.zh-CN.md`（对应两条）
- 修改：`docs/architecture-and-plugin-protocol.md`（工具目录与 Registry）

- [ ] **步骤 1：改文档**

Operator UI 列表改为：设置 → Tools 按 Connector / 路径前缀折叠，可搜索；可改显示名和说明（换 spec 保留人改）；添加在抽屉；`extra` 可删。

Tool catalog 节补三点（中英都要）：

- `title` 只出现在设置页，不进模型 tool list
- `PATCH` 可改 `title` / `description`；人改过的 `description` 带 `description_custom`，再 PUT spec 不覆盖
- 设置页树与搜索；组启停是多次 `PATCH enabled`，不是新接口

架构草案 `store.Tool` 字段列表加上 `title` / `description_custom`。合并规则加一句：`title` 始终保留；`description_custom=true` 时说明保留。

不要写批量目录 API，不要写用模型生成摘要。

- [ ] **步骤 2：回归**

```powershell
go test ./... -count=1
cd web/chat; npm test
```

预期：全绿。`tests/integration` 开箱路径不得因默认停用或文案字段失败。

- [ ] **步骤 3：Commit**

```powershell
git add README.md README.zh-CN.md docs/architecture-and-plugin-protocol.md
git commit -m "docs(tools): 设置页树形目录与可读文案"
```

---

## 自检对照规格

| 规格 | 任务 |
|------|------|
| §1.1–1.3 树 / 搜索 / 行展示 | 4、5 |
| §1.4–1.6 PATCH 文案、custom 合并、Registry | 2、3 |
| §1.7 添加抽屉 | 5 |
| §1.8 组头逐条 PATCH | 5 |
| §1.9 extra 删除与 ACL | 已有；5 保持 |
| §1.10 开箱集成 | 6 |
| §4 SQLite 列与迁移 | 1 |
| §5 MergeCatalog | 2 |
| §6.2 只改 title 不碰 Registry；改说明 SetDescription | 3 |
| §6.3 extra `description_custom=true` | 3 |
| §8 组启停部分失败汇总 | 5 |
| §10 README / 架构 | 6 |
| 不做批量接口、不用模型摘要 | 全局约束；文档禁止出现 |

类型名全程：`Title`、`DescriptionCustom`、`SetDescription`、`pathPrefixGroup`、`toolMatchesQuery`、`groupToolsTree`。
