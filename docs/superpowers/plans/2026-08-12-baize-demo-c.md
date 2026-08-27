# Baize Demo C 实现计划（OpenAPI 接入契约）

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 平台可通过 PUT/GET Connector 与 GET Tools 看清 OpenAPI 接入契约；换 Spec 可替换；坏 Spec / 同名冲突有明确错误；`/ui` 只读展示 Tools；mock-ticket 更像遗留系统。

**架构：** 在现有 Registry 上为每个 Tool 挂 `connector_id` + HTTP 映射元数据；PUT 时先冲突检测再按 connector 替换；控制面补齐查询 API；UI 调 `GET /v0/tools`；样板与文档服务平台叙事。不引入侧车进程。

**技术栈：** Go 1.22+、现有 kin-openapi、Vite + TypeScript（`web/chat`）、httptest。

**规格：** `docs/superpowers/specs/2026-08-12-baize-demo-c-design.md`

---

## 文件结构（将创建/修改）

| 路径 | 职责 |
|------|------|
| `internal/tool/registry.go` | Tool 元数据、`List()`、按 connector 卸载、跨 connector 同名冲突检测 |
| `internal/tool/registry_test.go` | 上列行为的单测 |
| `internal/connector/openapi/loader.go` | `ToolRoute.OperationID` 显式字段（与 Name 同源时拷贝） |
| `internal/connector/openapi/register.go` | **新建**：`RegisterConnector(st, reg, id, spec, baseURL, requireApproval)` 统一 PUT/demo 注册路径 |
| `internal/store/store.go` | `Connector.RequireApproval []string`；可选 `ToolNames` 不落库（以 Registry 为准） |
| `internal/api/server.go` | PUT 响应用摘要；GET connector/tools；409；走统一 Register |
| `internal/api/server_test.go` | GET、冲突、坏 Spec、PUT 响应 tools |
| `internal/demo/demo.go` | 改用统一 Register |
| `examples/mock-ticket/server.go` | `GET /tickets/{id}`、`PATCH /tickets/{id}`（改 status） |
| `examples/mock-ticket/openapi.yaml` | 对应 operation + 4xx 示意 |
| `examples/mock-ticket/server_test.go` | 新路由测试 |
| `web/chat/src/api.ts` | `listTools()` |
| `web/chat/src/main.ts` + `style.css` | 只读 Tools 面板 |
| `internal/ui/dist/**` | `npm run build` 后提交产物 |
| `tests/integration/connector_contract_test.go` | **新建**：冷启动列 Tools、替换、坏 Spec |
| `README.md` | 「平台接入三步」一节 |
| `docs/architecture-and-plugin-protocol.md` | 控制面表格补 GET（短改） |

---

### 任务 1：Registry 元数据 + 冲突 + 按 Connector 卸载

**文件：**
- 修改：`internal/tool/registry.go`
- 修改：`internal/tool/registry_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
func TestRegistryListAndConnectorReplace(t *testing.T) {
	r := tool.NewRegistry()
	nop := func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{}, false, nil
	}
	r.RegisterMeta(tool.Meta{
		Spec: llm.ToolSpec{Name: "create_ticket", Description: "c"},
		ConnectorID: "ticket", OperationID: "create_ticket",
		Method: "POST", Path: "/tickets",
	}, nop, true)

	infos := r.List()
	if len(infos) != 1 || infos[0].Method != "POST" || infos[0].ConnectorID != "ticket" {
		t.Fatalf("list=%+v", infos)
	}

	// 另一 connector 同名 → 冲突
	if !r.WouldConflict("other", []string{"create_ticket"}) {
		t.Fatal("expected conflict")
	}

	r.UnregisterConnector("ticket")
	if len(r.List()) != 0 {
		t.Fatal("expected empty after unregister connector")
	}
}
```

- [ ] **步骤 2：运行确认失败**

```bash
go test ./internal/tool/ -run TestRegistryListAndConnectorReplace -count=1
```

预期：FAIL（`RegisterMeta` / `List` / `WouldConflict` / `UnregisterConnector` 未定义）

- [ ] **步骤 3：最少实现**

在 `registry.go` 增加：

```go
type Meta struct {
	Spec            llm.ToolSpec
	ConnectorID     string
	OperationID     string
	Method          string
	Path            string
}

type Info struct {
	Name            string         `json:"name"`
	Description     string         `json:"description,omitempty"`
	ConnectorID     string         `json:"connector_id"`
	OperationID     string         `json:"operation_id,omitempty"`
	Method          string         `json:"method,omitempty"`
	Path            string         `json:"path,omitempty"`
	InputSchema     map[string]any `json:"input_schema,omitempty"`
	RequireApproval bool           `json:"require_approval"`
}

// entry 增加 connectorID, operationID, method, path 字段
// RegisterMeta 写入上述字段
// List() []Info 按 name 排序
// WouldConflict(connectorID string, names []string) bool
//   —— 若某 name 已存在且其 connectorID != 入参 connectorID（且非空）则 true
// UnregisterConnector(connectorID string) —— 删除所有该 connector 的 tool
```

保留现有 `Register` / `RegisterSpecApproved`（connectorID 为空，供单测）；`Specs()` 行为不变。

- [ ] **步骤 4：跑通测试**

```bash
go test ./internal/tool/ -count=1
```

预期：PASS

- [ ] **步骤 5：Commit**

```bash
git add internal/tool/registry.go internal/tool/registry_test.go
git commit -m "$(cat <<'EOF'
feat(tool): Tool 元数据、List 与按 Connector 卸载

EOF
)"
```

（Windows PowerShell 可用：`git commit -m "feat(tool): Tool 元数据、List 与按 Connector 卸载"`）

---

### 任务 2：OpenAPI Loader OperationID + 统一 RegisterConnector

**文件：**
- 修改：`internal/connector/openapi/loader.go`
- 修改：`internal/connector/openapi/loader_test.go`（断言 OperationID）
- 创建：`internal/connector/openapi/register.go`
- 创建：`internal/connector/openapi/register_test.go`
- 修改：`internal/store/store.go`（`Connector.RequireApproval`）

- [ ] **步骤 1：失败测试 — OperationID**

```go
func TestLoadToolsOperationID(t *testing.T) {
	tools, err := openapi.LoadTools("testdata/...") // 或沿用现有 fixture
	// 找到 create_ticket：OperationID == "create_ticket" && Method == "POST"
}
```

- [ ] **步骤 2：Loader 写入 `OperationID`**

```go
type ToolRoute struct {
	Name        string
	OperationID string
	Description string
	Method      string
	Path        string
	InputSchema map[string]any
}
// name := op.OperationID; if empty { name = normalize... }
// OperationID: op.OperationID（可为空；Name 仍为可调用名）
```

- [ ] **步骤 3：失败测试 — RegisterConnector 冲突与替换**

```go
func TestRegisterConnectorConflictAndReplace(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	// 先注册 connector A 含 create_ticket
	// 再注册 connector B 同名 → error 类型可检查 / 返回 err 含 conflict
	// 同 id 再 Register 换 Spec → 旧 tool 消失、新 tool 出现
}
```

- [ ] **步骤 4：实现 `RegisterConnector`**

```go
func RegisterConnector(
	st store.Store,
	reg *tool.Registry,
	id, typ, specPath, baseURL string,
	requireApproval []string,
) (store.Connector, []tool.Info, error) {
	if typ == "" {
		typ = "openapi"
	}
	if typ != "openapi" {
		return store.Connector{}, nil, fmt.Errorf("unsupported connector type")
	}
	routes, err := LoadTools(specPath)
	if err != nil {
		return store.Connector{}, nil, fmt.Errorf("invalid_spec: %w", err)
	}
	names := make([]string, len(routes))
	for i, r := range routes {
		names[i] = r.Name
	}
	if reg.WouldConflict(id, names) {
		return store.Connector{}, nil, errToolConflict // 或 errors.New("tool_conflict")
	}
	reg.UnregisterConnector(id)
	approval := map[string]bool{}
	for _, n := range requireApproval {
		approval[n] = true
	}
	inv := &Invoker{BaseURL: baseURL, Tools: routes}
	for _, route := range routes {
		route := route
		name := route.Name
		reg.RegisterMeta(tool.Meta{
			Spec: llm.ToolSpec{
				Name: route.Name, Description: route.Description, InputSchema: route.InputSchema,
			},
			ConnectorID: id, OperationID: route.OperationID,
			Method: route.Method, Path: route.Path,
		}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
			res, err := inv.Invoke(ctx, name, args)
			if err != nil {
				return nil, true, err
			}
			return res.Content, res.IsError, nil
		}, approval[name])
	}
	c := store.Connector{
		ID: id, Type: typ, Spec: specPath, BaseURL: baseURL,
		RequireApproval: requireApproval,
	}
	st.UpsertConnector(c)
	return c, filterInfos(reg, id), nil
}
```

`store.Connector` 增加：

```go
RequireApproval []string `json:"require_approval,omitempty"`
```

- [ ] **步骤 5：测试通过 + Commit**

```bash
go test ./internal/connector/openapi/ ./internal/store/ -count=1
git add internal/connector/openapi/ internal/store/store.go
git commit -m "feat(openapi): RegisterConnector 与 OperationID 元数据"
```

---

### 任务 3：控制面 GET/PUT 硬化

**文件：**
- 修改：`internal/api/server.go`
- 修改：`internal/api/server_test.go`
- 修改：`internal/demo/demo.go`（改用 `openapi.RegisterConnector`）

- [ ] **步骤 1：写失败 API 测试**

```go
func TestGetToolsAndConnector(t *testing.T) { /* PUT 后 GET /v0/tools 含 method/path；GET /v0/connectors/ticket 200 */ }
func TestPutConnectorToolConflict(t *testing.T) {
	// 两个不同 id、同一 spec（同名 tools）→ 第二次 PUT 409 tool_conflict
}
func TestPutConnectorInvalidSpecLeavesRegistry(t *testing.T) {
	// 先成功 PUT；再 PUT 坏路径 → 400；GET /v0/tools 仍为第一次的 tools
}
func TestPutConnectorResponseIncludesTools(t *testing.T) {
	// 响应 JSON 含 tools 数组
}
```

- [ ] **步骤 2：实现路由与 handler**

```go
s.mux.HandleFunc("GET /v0/connectors/{id}", s.handleGetConnector)
s.mux.HandleFunc("GET /v0/tools", s.handleGetTools)
```

`handlePutConnector`：
- 解析 `require_approval`（可空）
- 调用 `openapi.RegisterConnector`
- 若 err 含 `invalid_spec` → 400；`tool_conflict` → 409
- 成功响应：

```json
{
  "id": "...",
  "type": "openapi",
  "spec": "...",
  "base_url": "...",
  "require_approval": ["create_ticket"],
  "tools": [ /* tool.Info 列表 */ ]
}
```

`handleGetConnector`：Store 取 Connector + 过滤 `reg.List()` 中同 connector_id 的 name 列表（或内嵌 tools 摘要）。

`handleGetTools`：`writeJSON(200, map[string]any{"tools": s.Registry.List()})`

- [ ] **步骤 3：`demo.registerConnector` 改为调用 `openapi.RegisterConnector`**，删除重复注册循环。

- [ ] **步骤 4：测试**

```bash
go test ./internal/api/ ./internal/demo/ -count=1
go test ./... -count=1
```

- [ ] **步骤 5：Commit** `feat(api): GET connectors/tools 与 PUT 冲突语义`

---

### 任务 4：加重 mock-ticket 遗留样板

**文件：**
- 修改：`examples/mock-ticket/server.go`
- 修改：`examples/mock-ticket/openapi.yaml`
- 修改：`examples/mock-ticket/server_test.go`
- 修改：`configs/demo.yaml` / `configs/demo.local.yaml`（若 approval 列表需含新危险工具则加；默认仍审批 `create_ticket`，改状态可审批或否——**默认对 `update_ticket_status` 也 require_approval**）

- [ ] **步骤 1：失败测试**

```go
func TestGetTicketByID(t *testing.T) { /* create → GET /tickets/{id} → 200；未知 id → 404 */ }
func TestPatchTicketStatus(t *testing.T) {
	/* PATCH {"status":"closed"} → 200；非法 status → 400 */
}
```

- [ ] **步骤 2：实现**

`Ticket` 增加 `Status string`（默认 `"open"`）。

```go
mux.HandleFunc("GET /tickets/{id}", s.handleGetTicket)
mux.HandleFunc("PATCH /tickets/{id}", s.handlePatchTicket)
```

- [ ] **步骤 3：更新 openapi.yaml**

```yaml
/tickets/{id}:
  get:
    operationId: get_ticket
    parameters: [{ name: id, in: path, required: true, schema: { type: string } }]
    responses:
      "200": { description: ok }
      "404": { description: not found }
  patch:
    operationId: update_ticket_status
    parameters: [{ name: id, in: path, required: true, schema: { type: string } }]
    requestBody:
      required: true
      content:
        application/json:
          schema:
            type: object
            required: [status]
            properties:
              status: { type: string }
    responses:
      "200": { description: ok }
      "400": { description: bad request }
      "404": { description: not found }
```

确认现有 create/list 保留；`list`/`create` 的 responses 可补 `"400"` 示意。

- [ ] **步骤 4：确认 OpenAPI loader 能加载 path 参数工具**（若 invoke 尚不支持 path 参数，**最少**：在 `openapi/invoke.go` 用 args 中的 `id` 替换 `{id}`；为 get/patch 写 loader+invoke 单测）。

- [ ] **步骤 5：测试 + Commit**

```bash
go test ./examples/mock-ticket/ ./internal/connector/openapi/ -count=1
git commit -m "feat(mock-ticket): 按 id 查询与改状态，补强 OpenAPI"
```

---

### 任务 5：Chat UI 只读 Tools 面板 + 构建产物

**文件：**
- 修改：`web/chat/src/api.ts`
- 修改：`web/chat/src/main.ts`
- 修改：`web/chat/src/style.css`
- 修改：`internal/ui/dist/**`（build 输出）

- [ ] **步骤 1：api.ts**

```ts
export interface ToolInfo {
  name: string
  description?: string
  connector_id: string
  operation_id?: string
  method?: string
  path?: string
  require_approval?: boolean
}

export async function listTools(): Promise<ToolInfo[]> {
  const res = await fetch('/v0/tools')
  const body = await parseJSON<{ tools: ToolInfo[] }>(res)
  return body.tools ?? []
}
```

- [ ] **步骤 2：UI** — 在 header 下或侧栏增加 `<aside id="tools-panel">`，页面加载时 `listTools()` 渲染表格/列表：`METHOD path — name (connector)`；失败时显示「无法加载 Tools」。不做编辑。

- [ ] **步骤 3：样式** — 紧凑只读面板，不抢对话主区域（窄条或可折叠；默认展开）。

- [ ] **步骤 4：构建并嵌入**

```bash
cd web/chat
npm ci --registry=https://registry.npmmirror.com
npm run build
```

确认 `internal/ui/dist` 更新。

- [ ] **步骤 5：Commit** `feat(ui): 只读 Tools 面板展示接入契约`

---

### 任务 6：集成测试 + 文档

**文件：**
- 创建：`tests/integration/connector_contract_test.go`
- 修改：`README.md`
- 修改：`docs/architecture-and-plugin-protocol.md`（控制面表增加 GET）

- [ ] **步骤 1：集成测试**（沿用 `demo.StartForTest`，LLM=mock）

```go
func TestConnectorContractListReplaceAndBadSpec(t *testing.T) {
	// StartForTest
	// GET /v0/tools → 非空，含 method/path
	// PUT 同 id 换另一份临时 spec 文件（少一个 operation）→ tools 变少
	// PUT 坏 spec 路径 → 400；再次 GET tools 与替换后一致（未被破坏）
}
```

- [ ] **步骤 2：跑测试**

```bash
go test ./tests/integration/ -count=1
go test ./... -count=1
```

- [ ] **步骤 3：README 增加「平台接入（OpenAPI）」**

固定四步：

1. 准备 OpenAPI + 可达 `base_url`  
2. `PUT /v0/connectors/{id}`（示例 curl）  
3. `GET /v0/tools` 核对  
4. `POST /v0/runs` 或打开 `/ui`  

注明：主路径 OpenAPI；无 Swagger 见后续侧车；鉴权深做不在本里程碑。

- [ ] **步骤 4：架构文档控制面表补两行 GET**

- [ ] **步骤 5：Commit** `docs+test(demo-c): 接入契约集成测试与平台文档`

---

## 自检（对照规格）

| 规格要点 | 任务 |
|----------|------|
| PUT 响应 tools[]、require_approval | 3 |
| GET connector / GET tools + 元数据字段 | 1, 3 |
| 409 tool_conflict；替换语义；坏 Spec 不污染 | 1, 2, 3, 6 |
| mock-ticket 加重 + operationId | 4 |
| 接入文档三步 | 6 |
| /ui 只读 Tools 面板 | 5 |
| 不做侧车/鉴权深做/Agent 白名单 | 全计划未包含 |

无 TODO/待定占位；类型名统一为 `tool.Meta` / `tool.Info` / `openapi.RegisterConnector`。

---

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-12-baize-demo-c.md`。

**两种执行方式：**

1. **子代理驱动（推荐）** — 每任务新子代理 + 任务间审查  
2. **内联执行** — 本会话用 executing-plans 按任务推进并设检查点  

选哪种方式？
