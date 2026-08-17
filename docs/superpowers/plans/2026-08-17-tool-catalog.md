# Connector 工具目录 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** Connector 拥有可落盘的 Tool 目录：管理员可启停；OpenAPI 可手加同一 `base_url` 的 REST；插件只能启停；换 Spec / 重启不丢勾选和手加。

**架构：** `store.Tool` 为第一等公民，SQLite 落盘 Connector + Tool。`connector.MergeCatalog` 合并发现结果与已有行。`Apply` 只把 `enabled=true` 的行写入 Registry。控制面读目录；引擎仍只看 Registry。

**技术栈：** Go 1.22+、现有 httptest、React + Vite、vitest。不新加依赖。

**规格：** `docs/superpowers/specs/2026-08-17-tool-catalog-design.md`

**全局约束：**
- 不在 `main` 上改实现代码：先 `git checkout -b feat/tool-catalog`
- 不改会话身份优先级、三种 `auth.mode`、HITL 语义、控制面口令
- 不做 Agent 白名单、MCP、插件手加 REST、PATCH `require_approval`
- commit 中文 `type(scope): 说明`；PowerShell 不要 bash HEREDOC
- Go：`C:\Users\Administrator\sdk\go\bin`；`GOPROXY=https://goproxy.cn,direct`
- 每步测试：`$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH; go test <pkg> -count=1`

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/store/store.go` | `Tool`、`ToolSource*`、`ListConnectors` / `ListTools` / `GetTool` / `UpsertTool` / `DeleteTool` / `ReplaceConnectorTools` |
| `internal/store/memory.go` | 内存实现 |
| `internal/store/sqlite.go` | `connectors` + `tools` 表 |
| `internal/store/store_test.go` / `sqlite_test.go` | 目录 CRUD 与落盘 |
| `internal/connector/catalog.go` | `MergeCatalog` |
| `internal/connector/catalog_test.go` | 合并表驱动 |
| `internal/connector/apply.go` | 发现 → 合并 → 落盘 → 只注册启用行（含 extra Invoker） |
| `internal/connector/apply_test.go` | PUT 省略保留停用/extra；失败不脏写 |
| `internal/bootstrap/bootstrap.go` | YAML Apply 之后加载库中其余 Connector |
| `internal/bootstrap/catalog_test.go` | sqlite 重启后 extra/停用仍在 |
| `internal/api/server.go` | GET 目录；PATCH `enabled`；POST/DELETE extra |
| `internal/api/server_catalog_test.go` | 规格 §1 的 HTTP 条 |
| `internal/controlplane/acl.go` / `acl_test.go` | 新路由管理员 |
| `web/chat/src/api.ts` | `enabled`/`source`；patch enabled；POST/DELETE extra |
| `web/chat/src/toolCatalog.ts` | `canDeleteCatalogTool` |
| `web/chat/src/pages/ToolsSettings.tsx` | 启用开关、添加表单、删除 extra |
| `README.md` / `README.zh-CN.md` / `docs/architecture-and-plugin-protocol.md` | 文档 |
| `internal/ui/dist/**` | `npm run build` 产物 |

---

### 任务 1：Store 目录行与 SQLite 落盘

**文件：**
- 修改：`internal/store/store.go`
- 修改：`internal/store/memory.go`
- 修改：`internal/store/sqlite.go`
- 修改：`internal/store/store_test.go`
- 修改：`internal/store/sqlite_test.go`

- [ ] **步骤 1：写失败测试**

在 `store_test.go` 追加：

```go
func TestToolCatalogCRUD(t *testing.T) {
	s := store.NewMemory()
	s.UpsertConnector(store.Connector{ID: "c1", Type: "openapi", Spec: "s.yaml", BaseURL: "http://x"})
	row := store.Tool{
		ConnectorID: "c1",
		Name:        "create_ticket",
		Source:      store.ToolSourceSpec,
		Enabled:     true,
		Method:      "POST",
		Path:        "/tickets",
		Description: "create",
		InputSchema: map[string]any{"type": "object"},
	}
	s.UpsertTool(row)
	got, err := s.GetTool("create_ticket")
	if err != nil || !got.Enabled || got.Source != store.ToolSourceSpec {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	list := s.ListTools()
	if len(list) != 1 || list[0].Name != "create_ticket" {
		t.Fatalf("list=%+v", list)
	}
	byC := s.ListToolsByConnector("c1")
	if len(byC) != 1 {
		t.Fatalf("byC=%+v", byC)
	}
	row.Enabled = false
	s.UpsertTool(row)
	got, _ = s.GetTool("create_ticket")
	if got.Enabled {
		t.Fatal("expected disabled")
	}
	if err := s.DeleteTool("create_ticket"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTool("create_ticket"); err == nil {
		t.Fatal("expected not found")
	}
	cs := s.ListConnectors()
	if len(cs) != 1 || cs[0].ID != "c1" {
		t.Fatalf("connectors=%+v", cs)
	}
}

func TestReplaceConnectorToolsKeepsOthers(t *testing.T) {
	s := store.NewMemory()
	s.UpsertTool(store.Tool{ConnectorID: "a", Name: "keep", Source: store.ToolSourceSpec, Enabled: true})
	s.UpsertTool(store.Tool{ConnectorID: "b", Name: "gone", Source: store.ToolSourceSpec, Enabled: true})
	s.ReplaceConnectorTools("b", []store.Tool{
		{ConnectorID: "b", Name: "new", Source: store.ToolSourceExtra, Enabled: true, Method: "GET", Path: "/n"},
	})
	if _, err := s.GetTool("gone"); err == nil {
		t.Fatal("gone should be replaced away")
	}
	if _, err := s.GetTool("keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTool("new"); err != nil {
		t.Fatal(err)
	}
}
```

`sqlite_test.go` 追加：`OpenSQLite(temp)` → UpsertConnector + UpsertTool → Close → 再 Open → `GetTool` / `ListConnectors` 仍在。`InputSchema` JSON 往返。

- [ ] **步骤 2：跑测试确认失败**

```powershell
$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH
go test ./internal/store -count=1 -run "ToolCatalog|ReplaceConnector|SQLite.*Tool"
```

预期：FAIL，`ListTools` 未定义。

- [ ] **步骤 3：最少实现**

`store.go` 增加：

```go
const (
	ToolSourceSpec   = "spec"
	ToolSourceExtra  = "extra"
	ToolSourcePlugin = "plugin"
)

type Tool struct {
	ConnectorID     string         `json:"connector_id"`
	Name            string         `json:"name"`
	Source          string         `json:"source"`
	Enabled         bool           `json:"enabled"`
	Description     string         `json:"description,omitempty"`
	Method          string         `json:"method,omitempty"`
	Path            string         `json:"path,omitempty"`
	InputSchema     map[string]any `json:"input_schema,omitempty"`
	RequireLogin    bool           `json:"require_login"`
	RequireApproval bool           `json:"require_approval"`
	OperationID     string         `json:"operation_id,omitempty"`
}
```

`Store` 接口追加：`ListConnectors() []Connector`、`ListTools() []Tool`（按 name 排序）、`ListToolsByConnector(id string) []Tool`、`GetTool(name string) (Tool, error)`（未找到 `"tool not found"`）、`UpsertTool(Tool)`、`DeleteTool(name string) error`、`ReplaceConnectorTools(connectorID string, tools []Tool)`。

Memory：`tools map[string]Tool`。`ListConnectors` 遍历 `connectors` 按 id 排序。

SQLite：`CREATE TABLE IF NOT EXISTS connectors (id TEXT PRIMARY KEY, type TEXT, spec TEXT, base_url TEXT, require_approval_json TEXT, require_login_json TEXT, auth_json TEXT);` `CREATE TABLE IF NOT EXISTS tools (name TEXT PRIMARY KEY, connector_id TEXT, source TEXT, enabled INTEGER, description TEXT, method TEXT, path TEXT, input_schema_json TEXT, require_login INTEGER, require_approval INTEGER, operation_id TEXT);` Open 时建表；`UpsertConnector`/`GetConnector`/`ListConnectors` 走表不再只用内存 map（Open 后从 DB load 进 map 或每次查 DB，选「写 DB 且更新 map」与现有 runs 一致即可）。`OpenSQLite` 成功后 `loadConnectorsAndTools`。

- [ ] **步骤 4：跑测试确认通过**

```powershell
go test ./internal/store -count=1
```

预期：PASS。

- [ ] **步骤 5：Commit**

```powershell
git add internal/store
git commit -m "feat(store): Tool 目录行与 Connector SQLite 落盘"
```

---

### 任务 2：MergeCatalog

**文件：**
- 创建：`internal/connector/catalog.go`
- 创建：`internal/connector/catalog_test.go`

- [ ] **步骤 1：写失败测试**

```go
package connector

import (
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestMergeCatalogKeepsDisabledAndExtras(t *testing.T) {
	existing := []store.Tool{
		{Name: "old", ConnectorID: "c", Source: store.ToolSourceSpec, Enabled: false, RequireLogin: true},
		{Name: "gone", ConnectorID: "c", Source: store.ToolSourceSpec, Enabled: true},
		{Name: "extra1", ConnectorID: "c", Source: store.ToolSourceExtra, Enabled: true, Method: "GET", Path: "/e"},
	}
	discovered := []store.Tool{
		{Name: "old", ConnectorID: "c", Source: store.ToolSourceSpec, Method: "POST", Path: "/old", Description: "d"},
		{Name: "fresh", ConnectorID: "c", Source: store.ToolSourceSpec, Method: "GET", Path: "/n"},
	}
	out := MergeCatalog(MergeOpts{Existing: existing, Discovered: discovered})
	by := map[string]store.Tool{}
	for _, r := range out {
		by[r.Name] = r
	}
	if _, ok := by["gone"]; ok {
		t.Fatal("spec row missing from spec should drop")
	}
	if by["old"].Enabled || !by["old"].RequireLogin || by["old"].Path != "/old" {
		t.Fatalf("old=%+v", by["old"])
	}
	if !by["fresh"].Enabled || by["fresh"].RequireLogin {
		t.Fatalf("fresh=%+v", by["fresh"])
	}
	if by["extra1"].Path != "/e" {
		t.Fatalf("extra=%+v", by["extra1"])
	}
}

func TestMergeCatalogExplicitLoginRewrites(t *testing.T) {
	login := []string{"a"}
	out := MergeCatalog(MergeOpts{
		Existing: []store.Tool{
			{Name: "a", ConnectorID: "c", Source: store.ToolSourceSpec, Enabled: true, RequireLogin: false},
			{Name: "b", ConnectorID: "c", Source: store.ToolSourceSpec, Enabled: true, RequireLogin: true},
		},
		Discovered: []store.Tool{
			{Name: "a", ConnectorID: "c", Source: store.ToolSourceSpec},
			{Name: "b", ConnectorID: "c", Source: store.ToolSourceSpec},
		},
		RequireLogin: &login,
	})
	by := map[string]store.Tool{}
	for _, r := range out {
		by[r.Name] = r
	}
	if !by["a"].RequireLogin || by["b"].RequireLogin {
		t.Fatalf("%+v", by)
	}
}
```

再加一条：`RequireApproval: []string{"a"}` 写到行上；`source=plugin` 消失则删除 plugin 行、保留 extra。

- [ ] **步骤 2：跑测试确认失败**

```powershell
go test ./internal/connector -count=1 -run MergeCatalog
```

预期：FAIL，`MergeCatalog` 未定义。

- [ ] **步骤 3：最少实现**

```go
type MergeOpts struct {
	Existing            []store.Tool
	Discovered          []store.Tool
	RequireLogin        *[]string
	RequireApproval     []string
}

func MergeCatalog(opts MergeOpts) []store.Tool
```

算法：`existingByName`；结果先放入所有 `source==extra`；对 `discovered` 每个名字：若已有同名且 source 为 spec/plugin，保留 Enabled 与（当 RequireLogin==nil 时）RequireLogin；更新 method/path/description/source/operation_id；新名 Enabled=true，RequireLogin=false（除非名单命中）。不要把 discovered 里没有的 spec/plugin 放进结果。最后若 `RequireLogin != nil` 或 `RequireApproval` 非空：对结果中非 extra 以及仍在 discovered 的行按名单重写对应布尔（approval：名单里为 true；login：仅当指针非 nil）。**extra 行：RequireLogin 指针非 nil 时也按名单重写**（与「该 Connector 下整表」一致）；指针 nil 则 extra 门闸不动。

- [ ] **步骤 4：跑测试确认通过**

```powershell
go test ./internal/connector -count=1 -run MergeCatalog
```

预期：PASS。

- [ ] **步骤 5：Commit**

```powershell
git add internal/connector/catalog.go internal/connector/catalog_test.go
git commit -m "feat(connector): 合并 OpenAPI/插件发现与已有 Tool 目录"
```

---

### 任务 3：Apply 只注册启用行；开机加载库中 Connector

**文件：**
- 修改：`internal/connector/apply.go`
- 修改：`internal/connector/openapi/register.go`（或抽出 `registerEnabled`：Invoker 含 spec 路由 + extra 路由）
- 修改：`internal/connector/httpplugin/register.go`（发现后合并，只注册 enabled）
- 修改：`internal/connector/apply_test.go`
- 修改：`internal/bootstrap/bootstrap.go`
- 创建：`internal/bootstrap/catalog_restart_test.go`

- [ ] **步骤 1：写失败测试**

`apply_test.go`：先 `Apply` 一份两 operation 的 spec；`UpsertTool` 把其中一个 `Enabled=false` 并加 extra 行；再 `Apply` 且 `RequireLogin: nil`。断言：`reg.Get` 停用名不存在；`st.GetTool` 停用仍在且 extra 在；`reg.List()` 含 extra。

坏 spec 的 Apply：预先有 tool 行，失败后行与 Registry 与失败前一致。

`catalog_restart_test.go`：`store.Open("sqlite", tmp)` → Apply 默认 connector → 停用一把 + extra → `Close` → 再 `newAPIServer` 等价路径（或直接 `registerConnector` + `loadStoredConnectors`）→ extra 与停用仍在。YAML 未写目录。

- [ ] **步骤 2：跑测试确认失败**

```powershell
go test ./internal/connector ./internal/bootstrap -count=1 -run "Catalog|Disabled|Restart"
```

预期：FAIL（Apply 仍整包注册，sqlite Connector 重启丢失）。

- [ ] **步骤 3：最少实现**

`Apply`：解析鉴权成功后，**先** `LoadTools` / 插件 list（失败则 return，不 `UnregisterConnector`）。构造 `discovered []store.Tool`。`existing := in.Store.ListToolsByConnector(in.ID)`。`merged := MergeCatalog(...)`。`WouldConflict`：用**其他** Connector 已有 tool 名（`ListTools`）加上 merged 名，不要只看 Registry（停用行也占名）。冲突或无效则 return，Store/Registry 不变。

然后：`ReplaceConnectorTools` + `UpsertConnector`（`RequireLogin`/`RequireApproval` 从 merged 聚合名字列表回显）。`UnregisterConnector` 后只为 `enabled` 行调用抽出的 `registerOne`（`RegisterMeta` + OpenAPI/插件闭包）。OpenAPI：`Invoker.Tools` = spec 路由 ∪ extra 转成的 `ToolRoute{Name, Method, Path, InputSchema, Description}`。插件：不停用的走现有 sidecar invoke；extra 不会出现在 plugin 合并结果里。任务 4 的 PATCH/POST 必须复用同一 `registerOne`，不得再复制一份闭包。

`bootstrap.newAPIServer`：`registerConnector`（YAML）之后 `for _, c := range st.ListConnectors() { if c.ID == cfg.Connector.ID { continue }; Apply 该行的 Type/Spec/BaseURL/Auth，RequireLogin: nil }`。Apply 内部会再合并目录，不得把 YAML 未写的停用冲掉。

聚合回显：

```go
func listsFromTools(tools []store.Tool) (login, approval []string) {
	for _, t := range tools {
		if t.RequireLogin {
			login = append(login, t.Name)
		}
		if t.RequireApproval {
			approval = append(approval, t.Name)
		}
	}
	sort.Strings(login)
	sort.Strings(approval)
	return
}
```

- [ ] **步骤 4：跑测试确认通过**

```powershell
go test ./internal/connector ./internal/bootstrap ./internal/store -count=1
```

预期：PASS。现有 Apply 捕获测试仍绿。

- [ ] **步骤 5：Commit**

```powershell
git add internal/connector internal/bootstrap
git commit -m "feat(connector): Apply 按目录启停注册并在开机恢复库中 Connector"
```

---

### 任务 4：控制面 API 与 ACL

**文件：**
- 修改：`internal/api/server.go`（`routes`、`handleGetTools`、`handleGetConnector`、`handlePatchTool`、新增 POST/DELETE）
- 创建：`internal/api/server_catalog_test.go`
- 修改：`internal/controlplane/acl.go`
- 修改：`internal/controlplane/acl_test.go`
- 修改：`internal/api/server_gate_test.go`（操作员 403 目录接口）

- [ ] **步骤 1：写失败测试**

`acl_test.go` 增加：

```go
{"POST", "/v0/connectors/c1/tools", RoleAdmin},
{"DELETE", "/v0/connectors/c1/tools/extra1", RoleAdmin},
```

`server_catalog_test.go`（`package api` 以便设口令，或 `api_test` 门关着）：

1. PUT OpenAPI（用现有测试 spec 夹具）→ GET `/v0/tools` 含 `enabled`+`source=spec`，条数=operation 数
2. PATCH `{"enabled":false}` → Registry 无此名；GET 仍有 `enabled:false`；PATCH `require_login` 仍可用
3. POST extra → 200；GET 含 `source=extra`；httptest 下游收到 method+path（可复用 mock-ticket 或局部 httptest）
4. DELETE spec 名 → 400 `invalid_request`；DELETE extra → 204，GET 无此名
5. 第二次 PUT 省略 `require_login`：停用与 extra 仍在；新 operation 启用
6. 插件 Connector POST extra → 400
7. 重名 POST → 409
8. 门开着操作员 POST/GET tools → 403；管理员 200

PATCH 两个字段都缺 → 400。GET `/v0/connectors/{id}` 的 `tools` 含停用行。

- [ ] **步骤 2：跑测试确认失败**

```powershell
go test ./internal/api ./internal/controlplane -count=1 -run "Catalog|MinRole"
```

预期：FAIL（路由不存在 / GET 不含停用）。

- [ ] **步骤 3：最少实现**

```go
s.mux.HandleFunc("POST /v0/connectors/{id}/tools", s.handlePostConnectorTool)
s.mux.HandleFunc("DELETE /v0/connectors/{id}/tools/{name}", s.handleDeleteConnectorTool)
```

`handleGetTools`：`writeJSON(..., map[string]any{"tools": s.Store.ListTools()})`。

`handleGetConnector`：`tools` 用 `ListToolsByConnector`，不要 `Registry.List` 过滤。

`handlePatchTool`：按名 `GetTool`（404 若无）；解码 `Enabled *bool` `RequireLogin *bool`；都 nil → 400；更新行 `UpsertTool`；`enabled` 从 true→false 则 `Registry.Unregister`；false→true 则按该 Connector 调用与 Apply 相同的单工具注册（抽 `registerOne(st, reg, connector, tool)`，避免复制闭包）。同步聚合 `c.RequireLogin` 后 `UpsertConnector`。响应为 Store 中的 `Tool` JSON（与 GET 元素相同，含 `enabled`/`source`）。

`handlePostConnectorTool`：GetConnector；type 不是 openapi → 400；校验 name/method/path/schema；`GetTool` 已存在 → 409；UpsertTool extra enabled true；registerOne。

`handleDeleteConnectorTool`：GetTool；source 不是 extra → 400；DeleteTool；Unregister。204 无 body。

ACL：

```go
{method: "POST", segments: []string{"v0", "connectors", "{id}", "tools"}, role: RoleAdmin},
{method: "DELETE", segments: []string{"v0", "connectors", "{id}", "tools", "{name}"}, role: RoleAdmin},
```

注意：`PUT /v0/connectors/{id}` 规则更短，匹配器按 **段数** 比较，不会误伤 POST `.../tools`。

- [ ] **步骤 4：跑测试确认通过**

```powershell
go test ./internal/api ./internal/controlplane ./internal/connector -count=1
```

预期：PASS。

- [ ] **步骤 5：Commit**

```powershell
git add internal/api internal/controlplane
git commit -m "feat(api): 工具目录 GET/PATCH/手加与删除"
```

---

### 任务 5：设置 → Tools GUI

**文件：**
- 创建：`web/chat/src/toolCatalog.ts`
- 创建：`web/chat/src/toolCatalog.test.ts`
- 修改：`web/chat/src/api.ts`
- 修改：`web/chat/src/pages/ToolsSettings.tsx`
- 修改：`web/chat/src/style.css`（表单间距，沿用 settings-* 类）
- 修改：`internal/ui/dist/**`（`npm run build`）

- [ ] **步骤 1：写失败测试**

`toolCatalog.test.ts`：

```ts
import { describe, expect, it } from 'vitest'
import { canDeleteCatalogTool } from './toolCatalog'

describe('canDeleteCatalogTool', () => {
  it('only extra', () => {
    expect(canDeleteCatalogTool('extra')).toBe(true)
    expect(canDeleteCatalogTool('spec')).toBe(false)
    expect(canDeleteCatalogTool('plugin')).toBe(false)
  })
})
```

- [ ] **步骤 2：跑测试确认失败**

```powershell
cd web/chat; npm test -- src/toolCatalog.test.ts
```

预期：FAIL，模块不存在。

- [ ] **步骤 3：最少实现**

```ts
export function canDeleteCatalogTool(source: string): boolean {
  return source === 'extra'
}
```

`ToolInfo` 增加 `enabled?: boolean; source?: string; input_schema?: Record<string, unknown>`。

```ts
export async function patchTool(name: string, body: { enabled?: boolean; require_login?: boolean }): Promise<ToolInfo>
export async function createConnectorTool(connectorId: string, body: {
  name: string; method: string; path: string; description?: string
  input_schema?: Record<string, unknown>
}): Promise<ToolInfo>
export async function deleteConnectorTool(connectorId: string, name: string): Promise<void>
```

`patchToolRequireLogin` 改为调用 `patchTool(name, { require_login })`。

`ToolsSettings.tsx`：每行增加启用 checkbox（调用 `patchTool`）；`canDeleteCatalogTool(t.source)` 时显示删除并 `deleteConnectorTool`；「添加工具」：connector 若列表里只有一个 openapi connector_id 就用它，若多条则下拉选 `connector_id`（从 tools 行收集，缺省第一个非 plugin 的 connector_id；若当前页只有 plugin 工具则隐藏添加）。字段：name、method select、path、description、schema textarea 默认 `{}`。提交 `createConnectorTool`。错误用现有 `settings-error`。

构建：

```powershell
cd web/chat; npm test; npm run build
```

把 `dist` 同步进 `internal/ui/dist`（现有 Chat 构建方式，不要手改 hash 文件名）。

- [ ] **步骤 4：跑测试确认通过**

```powershell
cd web/chat; npm test
```

预期：PASS。

- [ ] **步骤 5：Commit**

```powershell
git add web/chat internal/ui/dist
git commit -m "feat(ui): Tools 设置页启停、手加 REST 与删除 extra"
```

若 dist 极大，审查包排除 `internal/ui/dist/assets`，另附源码 diff（与控制面门禁任务 4 相同）。

---

### 任务 6：文档与开箱集成测试

**文件：**
- 修改：`README.md` / `README.zh-CN.md`
- 修改：`docs/architecture-and-plugin-protocol.md`
- 现有 `tests/integration` 不应改契约；跑全量确认

- [ ] **步骤 1：补文档**

README 中英设置 → Tools：启用/停用；OpenAPI 可补 REST；插件只能停用；SQLite 重启保留。写清与会话登录、控制面口令不是一回事。

架构草案：Tool 为 Connector 目录行；Registry 仅 `enabled`；Agent 用合集。

- [ ] **步骤 2：跑开箱测试**

```powershell
go test ./... -count=1
cd web/chat; npm test
```

预期：全部 PASS，含 `tests/integration` 四把 mock-ticket 工具。

- [ ] **步骤 3：Commit**

```powershell
git add README.md README.zh-CN.md docs/architecture-and-plugin-protocol.md
git commit -m "docs(tools): 说明 Connector 工具目录启停与手加"
```

---

## 自检

| 规格 | 任务 |
|------|------|
| §4 Tool 行、SQLite Connector+Tool | 1 |
| §5 合并 | 2–3 |
| 开机加载库中其它 Connector | 3 |
| §6 API、GET 含停用、PATCH enabled、POST/DELETE | 4 |
| §3.1 / §9 门禁 403 | 4 |
| §8 extra 调用同一 base_url / 会话头 | 3–4 |
| §7 GUI | 5 |
| §11 文档、§1.10 开箱 | 6 |
| 不做 Agent 白名单 / MCP / 插件 extra / PATCH 审批 | 未列入 |

类型名全程：`store.Tool`、`ToolSourceSpec|Extra|Plugin`、`MergeCatalog`、`MergeOpts`、`ReplaceConnectorTools`、`ListTools`、`registerOne`（若抽出）。

无 TODO/待定占位。
