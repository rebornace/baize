# P3-B 业务系统 / 插件设置页人话化 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 把「业务系统（OpenAPI）」与「插件（HTTP 插件）」两页改为基于 P3-A 原语的人话化「连接器外壳 + 两步编辑 Modal」，移除服务端默认凭证编辑，并让后端在 PUT 连接器时保留 Tools 页配置的登录捕获 capture。

**架构：** 新增展示型 `ConnectorShell`（页头/卡片列表/空态）与 `ConnectorEditorModal`（第 1 步连接信息、保存后第 2 步工具权限勾选）；纯逻辑抽到 `pages/connectorForms/{validate,permissions}.ts`；两页瘦身为加载 + 组合。后端在 `handlePutConnector` 中把 `auth.capture` 改为指针，省略时保留已有 capture，凭证字段照常按提交值（连接器页为空）写入。

**技术栈：** React 19 + TypeScript + Vitest（jsdom，`web/chat`）；Go 1.25 + `net/http` 测试（`internal/api`）。

**规格：** `docs/superpowers/specs/2026-09-11-webui-refresh-p3b-connector-pages-design.md`

**Git：** 已在分支 `feat/webui-p3b-connector-pages`；每个任务一个 commit。

**环境（Windows / PowerShell）：**

```powershell
$env:PATH = "C:\Users\Administrator\.local\go1.25.0\bin;" + $env:PATH
$env:GOTOOLCHAIN = "auto"
```

前端：

```powershell
cd web/chat
npm run test -- <pattern>   # 单测
npx tsc --noEmit            # 类型
npm run build               # 构建 + 重建 dist
```

后端：

```powershell
go test ./internal/api/... ./internal/connector/...
```

---

## 文件结构

| 文件 | 职责 | 动作 |
|---|---|---|
| `internal/api/server.go` | `authBody.Capture` 改指针；省略 capture 时用 `Store.GetConnector` 保留旧值 | 修改 |
| `internal/api/server_connector_capture_test.go` | capture 省略保留 / 新建默认 / 凭证清空 测试 | 新建 |
| `web/chat/src/strings.ts` | `CONNECTORS` 文案 + `connectorErrorText` 错误码中文化 | 修改 |
| `web/chat/src/strings.connector.test.ts` | 上述纯函数测试 | 新建 |
| `web/chat/src/pages/connectorForms/types.ts` | 表单态、字段错误、权限选择类型 | 新建 |
| `web/chat/src/pages/connectorForms/validate.ts` | 编号/服务地址/文档 纯校验 | 新建 |
| `web/chat/src/pages/connectorForms/permissions.ts` | 权限勾选 ⇄ 工具名列表、摘要文案 | 新建 |
| `web/chat/src/pages/connectorForms/*.test.ts` | 纯函数测试 | 新建 |
| `web/chat/src/components/settings/ConnectorShell.tsx` | 页头 + 卡片列表 + 空态 + 行「⋯」菜单 + 删除确认 | 新建 |
| `web/chat/src/components/settings/ConnectorShell.test.tsx` | 外壳展示测试 | 新建 |
| `web/chat/src/components/settings/ConnectorEditorModal.tsx` | 两步 Modal（连接信息 / 工具权限） | 新建 |
| `web/chat/src/components/settings/ConnectorEditorModal.test.tsx` | Modal 测试 | 新建 |
| `web/chat/src/styles/components.css` | 连接器卡片/权限列表少量样式 | 修改 |
| `web/chat/src/pages/PluginSettings.tsx` | 瘦身为插件页（加载 + 外壳 + Modal） | 改写 |
| `web/chat/src/pages/OpenApiSettings.tsx` | 瘦身为业务系统页（含文档上传/URL） | 改写 |
| `web/chat/src/pages/PluginSettings.test.ts`、`OpenApiSettings.test.ts` | 页面级集成测试（fetch mock） | 改写 |
| `web/chat/src/pages/captureForm.ts` | 删除仅旧鉴权表单使用的 `captureSummaryLabel` / `mergeAuthPreserveCapture` / `hasStoredCapture` | 修改 |
| `web/chat/src/pages/CaptureSettings.test.ts` | 删除上一函数对应测试块 | 修改 |

注意：`pages/connectorDelete.ts` 仍被 `McpSettings.tsx` 使用，本批**保留不动**（批次 C 迁移 MCP 时再收敛）；两页改用 `ConfirmDialog` 后不再 import 它。

---

### 任务 1：后端 PUT 连接器省略 capture 时保留旧值

**文件：**
- 修改：`internal/api/server.go`（`authBody` 约 952-969；`handlePutConnector` 约 844-868）
- 测试：`internal/api/server_connector_capture_test.go`（新建）

- [ ] **步骤 1：编写失败测试**

新建 `internal/api/server_connector_capture_test.go`：

```go
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// 两次 PUT 同一个 openapi 连接器：第一次显式设置自定义 capture；第二次请求体
// 完全不带 auth.capture（连接器页改版后的提交形状），capture 必须被保留，
// 同时 static 等默认凭证按第二次提交（空）写入。
func TestPutConnectorOmitsCapturePreservesExisting(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	spec := writeLoginGetMeAPISpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	srv.Identities = identity.NewMemoryStore()
	h := srv.Handler()

	first := map[string]any{
		"type":     "openapi",
		"spec":     spec,
		"base_url": upstream.URL,
		"auth": map[string]any{
			"mode": "static",
			"static": map[string]any{
				"headers": map[string]string{"Authorization": "Bearer OLD"},
			},
			"capture": map[string]any{
				"tool_name_glob":    "custom_login_*",
				"token_json_paths":  []string{"data.token"},
				"header_template":   "Token {{token}}",
			},
		},
	}
	putConnectorJSON(t, h, "cap1", first)

	// 第二次：连接器页提交，不含 auth（Go encoding/json 缺省即 nil 指针）。
	second := map[string]any{
		"type":     "openapi",
		"base_url": upstream.URL,
	}
	putConnectorJSON(t, h, "cap1", second)

	got, err := st.GetConnector("cap1")
	if err != nil {
		t.Fatalf("get connector: %v", err)
	}
	if got.Auth.Capture.ToolNameGlob != "custom_login_*" {
		t.Fatalf("capture glob not preserved: %+v", got.Auth.Capture)
	}
	if len(got.Auth.Capture.TokenJSONPaths) != 1 || got.Auth.Capture.TokenJSONPaths[0] != "data.token" {
		t.Fatalf("capture token paths not preserved: %+v", got.Auth.Capture)
	}
	if got.Auth.Capture.HeaderTemplate != "Token {{token}}" {
		t.Fatalf("capture template not preserved: %+v", got.Auth.Capture)
	}
	if len(got.Auth.Static.Headers) != 0 {
		t.Fatalf("static headers must be cleared on omit, got %+v", got.Auth.Static.Headers)
	}
}

// 新建连接器（无历史记录）且省略 capture：生效 CaptureDefaults（*login*）。
func TestPutConnectorNewOmitsCaptureGetsDefaults(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	h := srv.Handler()

	putConnectorJSON(t, h, "cap2", map[string]any{
		"type":     "openapi",
		"spec":     writeLoginGetMeAPISpec(t),
		"base_url": upstream.URL,
	})
	got, err := st.GetConnector("cap2")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Auth.Capture.ToolNameGlob != "*login*" {
		t.Fatalf("new connector capture default glob = %q want *login*", got.Auth.Capture.ToolNameGlob)
	}
}

// 显式提交 capture（含 __none__）必须覆盖旧值，不被保留逻辑吞掉。
func TestPutConnectorExplicitCaptureOverrides(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	h := srv.Handler()

	putConnectorJSON(t, h, "cap3", map[string]any{
		"type":     "openapi",
		"spec":     writeLoginGetMeAPISpec(t),
		"base_url": upstream.URL,
		"auth":     map[string]any{"capture": map[string]any{"tool_name_glob": "keep_*"}},
	})
	putConnectorJSON(t, h, "cap3", map[string]any{
		"type":     "openapi",
		"base_url": upstream.URL,
		"auth":     map[string]any{"capture": map[string]any{"tool_name_glob": "__none__"}},
	})
	got, _ := st.GetConnector("cap3")
	if got.Auth.Capture.ToolNameGlob != "__none__" {
		t.Fatalf("explicit capture not honored: %+v", got.Auth.Capture)
	}
}
```

同文件补充两个测试辅助函数（`apiNewServer` 与 `putConnectorJSON`；`writeLoginGetMeAPISpec` / `jsonBody` 已存在于 `server_test.go` 同包，可直接用）：

```go
import (
	"github.com/rebornace/baize/internal/api"
)

func apiNewServer(st store.Store, reg *tool.Registry) *api.Server {
	return api.NewServer(st, reg, &fakeRunner{store: st})
}

func putConnectorJSON(t *testing.T, h http.Handler, id string, body map[string]any) {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPut,
		"/v0/connectors/"+id, jsonBody(t, body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT %s status=%d body=%s", id, rr.Code, rr.Body.String())
	}
}
```

注意：`api` 已在本测试包其它用例中以 `api.NewServer` 调用（如 `server_test.go`），若该文件已有同名 helper，则复用、不要重复声明；把 import 合并进 import 块。

- [ ] **步骤 2：运行测试确认失败**

```powershell
$env:PATH = "C:\Users\Administrator\.local\go1.25.0\bin;" + $env:PATH
go test ./internal/api/ -run TestPutConnector -count=1
```

预期：`TestPutConnectorOmitsCapturePreservesExisting` FAIL（第二次 PUT 后 capture 被清空，glob 为空）。

- [ ] **步骤 3：把 `authBody.Capture` 改为指针**

在 `internal/api/server.go` 的 `authBody` 中：

```go
	Capture *struct {
		ToolNameGlob   string   `json:"tool_name_glob"`
		TokenJSONPaths []string `json:"token_json_paths"`
		LabelJSONPaths []string `json:"label_json_paths"`
		HeaderTemplate string   `json:"header_template"`
		DefaultScheme  string   `json:"default_scheme"`
	} `json:"capture"`
```

- [ ] **步骤 4：构造 `connectorAuth` 时实现省略保留**

替换 `handlePutConnector` 中构造 `store.ConnectorAuth` 的片段（现有 `var connectorAuth store.ConnectorAuth` 起的块）为：

```go
	var connectorAuth store.ConnectorAuth
	if body.Type != "mcp" {
		capture := store.CaptureAuth{}
		if body.Auth.Capture != nil {
			capture = store.CaptureAuth{
				ToolNameGlob:   body.Auth.Capture.ToolNameGlob,
				TokenJSONPaths: body.Auth.Capture.TokenJSONPaths,
				LabelJSONPaths: body.Auth.Capture.LabelJSONPaths,
				HeaderTemplate: body.Auth.Capture.HeaderTemplate,
				DefaultScheme:  body.Auth.Capture.DefaultScheme,
			}
		} else if existing, err := s.Store.GetConnector(id); err == nil {
			// 连接器页不再编辑 capture；省略时保留 Tools 页配置的登录捕获，
			// 避免一次普通保存把本人登录链路清空。
			capture = existing.Auth.Capture
		}
		connectorAuth = store.ConnectorAuth{
			Mode: body.Auth.Mode,
			Static: store.StaticAuth{
				Headers: body.Auth.Static.Headers,
			},
			Passthrough: store.PassThruAuth{
				Headers: body.Auth.Passthrough.Headers,
			},
			VaultRef: store.VaultRefAuth{
				Headers: body.Auth.VaultRef.Headers,
			},
			Capture: capture,
		}
	}
```

- [ ] **步骤 5：运行测试验证通过**

```powershell
go test ./internal/api/ -run TestPutConnector -count=1
go test ./internal/api/... ./internal/connector/... -count=1
```

预期：新增 3 个测试 PASS；既有 `TestPutConnectorWiresCaptureWithoutRestart`、`TestPutConnectorNoneCaptureDisables`、`TestPutHTTPPluginPreservesCaptureAndUsesIdentity` 仍 PASS。

- [ ] **步骤 6：格式化并 Commit**

```powershell
gofmt -w internal/api/server.go internal/api/server_connector_capture_test.go
git add internal/api/server.go internal/api/server_connector_capture_test.go
git commit --trailer "Co-authored-by: Cursor <cursoragent@cursor.com>" -m "feat(api): PUT 连接器省略 capture 时保留既有登录捕获配置"
```
---

### 任务 2：连接器文案与错误码中文化

**文件：**
- 修改：`web/chat/src/strings.ts`
- 测试：`web/chat/src/strings.connector.test.ts`（新建）

- [ ] **步骤 1：编写失败测试**

新建 `web/chat/src/strings.connector.test.ts`：

```ts
import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import { CONNECTORS, connectorErrorText, permissionSummary } from './strings'

describe('connectorErrorText', () => {
  it('maps known connector codes', () => {
    expect(connectorErrorText(new ApiError(400, 'invalid_spec', 'x'))).toContain('接口文档无法解析')
    expect(connectorErrorText(new ApiError(400, 'invalid_spec_url', 'x'))).toContain('文档链接格式')
    expect(connectorErrorText(new ApiError(400, 'spec_fetch_blocked', 'x'))).toContain('安全策略')
    expect(connectorErrorText(new ApiError(400, 'spec_fetch_failed', 'x'))).toContain('无法下载')
    expect(connectorErrorText(new ApiError(400, 'unsupported_import_format', 'x'))).toContain('不支持的文档格式')
    expect(connectorErrorText(new ApiError(400, 'invalid_plugin', 'x'))).toContain('插件')
    expect(connectorErrorText(new ApiError(409, 'tool_conflict', 'x'))).toContain('重名')
    expect(connectorErrorText(new ApiError(400, 'invalid_auth', 'x'))).toContain('凭证')
  })

  it('maps known plain validation messages', () => {
    expect(connectorErrorText(new ApiError(400, 'invalid_request', 'base_url is required'))).toContain('服务地址')
    expect(connectorErrorText(new ApiError(400, 'invalid_request', 'spec is required'))).toContain('接口文档')
  })

  it('falls back to a generic title for unknown codes', () => {
    const out = connectorErrorText(new ApiError(400, 'weird_code', 'boom'))
    expect(out.title).toBe(CONNECTORS.errorGeneric)
    expect(out.detail).toBe('weird_code: boom')
  })
})

describe('permissionSummary', () => {
  it('renders nothing when no tools gated', () => {
    expect(permissionSummary([], [])).toBeNull()
  })
  it('renders login and approval counts', () => {
    expect(permissionSummary(['a', 'b'], ['c'])).toBe('2 个工具需本人登录 · 1 个需审批')
  })
  it('renders login only', () => {
    expect(permissionSummary(['a'], [])).toBe('1 个工具需本人登录')
  })
})
```

- [ ] **步骤 2：运行测试确认失败**

```powershell
cd web/chat
npm run test -- src/strings.connector.test.ts
```

预期：FAIL（`connectorErrorText` / `permissionSummary` 未导出）。

- [ ] **步骤 3：在 `strings.ts` 追加文案与函数**

在 `strings.ts` 末尾追加（保持文件现有 `import { ApiError } from './api'` 已存在；若 `FriendlyError` 已定义则复用其类型）：

```ts
// ---- 设置页：业务系统 / 插件（连接器） ----
export const CONNECTORS = {
  openapiTitle: '业务系统',
  openapiDesc:
    '上传一份接口文档，助手就能调用公司的订单、工单等业务系统。无需写代码。',
  pluginTitle: '插件服务',
  pluginDesc: '接入你们自行部署、按白泽约定提供能力的程序，助手即可使用其能力。',
  addOpenapi: '接入业务系统',
  addPlugin: '接入插件服务',
  editOpenapi: '编辑业务系统',
  editPlugin: '编辑插件服务',
  openapiEmptyTitle: '还没有接入业务系统',
  openapiEmptyDesc: '上传一份接口文档，助手就能调用订单、工单等系统。',
  pluginEmptyTitle: '还没有接入插件服务',
  pluginEmptyDesc: '接入自行部署、按白泽约定提供能力的程序后，助手即可使用其能力。',
  menuEdit: '编辑',
  menuDelete: '删除',
  deleteTitle: '删除这个连接？',
  deleteBody: '删除后，该连接提供的工具会从助手的能力中移除。',
  deleteOk: '删除',
  saved: '已保存',
  deleted: '已删除',
  toolsLink: '查看工具',
  toolCount: (n: number) => `${n} 个工具`,
  stepInfo: '连接信息',
  stepPermissions: '工具权限',
  fieldId: '连接编号',
  fieldIdHint: '仅用于区分，保存后不可改；用小写字母、数字、- 或 _。',
  fieldBaseUrl: '服务地址',
  fieldSpec: '接口文档',
  fieldSpecHint: '上传文件（.json / .yaml / .yml），或在下方填写文档链接，二选一。',
  fieldSpecUrl: '文档链接',
  fieldFormat: '接口文档格式',
  fmtAuto: '自动识别',
  fmtOpenapi3: 'OpenAPI 3',
  fmtSwagger2: 'Swagger 2',
  fmtPostman: 'Postman 合集',
  specFileChosen: (name: string) => `已选择文件：${name}`,
  permsIntro:
    '勾选「需本人登录」后，每位运营用自己的账号访问，权限互不混用；不勾则任何人都能直接调用此工具。',
  permLogin: '使用前需本人登录',
  permApproval: '使用前需人工审批',
  nextToPermissions: '下一步：设置工具权限',
  save: '保存连接',
  saving: '正在保存…',
  skip: '暂不设置',
  finish: '完成',
  back: '上一步',
  cancel: '取消',
  errIdRequired: '请填写连接编号',
  errIdPattern: '连接编号只能用小写字母开头，后跟小写字母、数字、- 或 _（最长 64 位）',
  errBaseUrlRequired: '请填写服务地址',
  errBaseUrlHttp: '服务地址需以 http:// 或 https:// 开头',
  errSpecRequired: '请上传接口文档或填写文档链接',
  errorGeneric: '操作未能完成，请稍后重试。',
} as const

const CONNECTOR_CODE_TITLES: Record<string, string> = {
  invalid_spec: '接口文档无法解析，请确认是有效的 OpenAPI / Swagger / Postman 文档。',
  invalid_spec_url: '文档链接格式不正确，请检查链接。',
  spec_fetch_blocked: '服务器不允许抓取该地址的文档（地址被安全策略拦截）。',
  spec_fetch_failed: '无法下载该文档链接，请确认地址可以访问。',
  unsupported_import_format: '不支持的文档格式。',
  invalid_plugin: '无法从该插件地址识别到可用能力，请检查服务是否正常。',
  tool_conflict: '有工具与其它连接重名，请调整对方系统里的操作名称后重试。',
  invalid_auth: '连接保存的凭证无效，请联系管理员通过配置处理。',
}

/** 把连接器相关异常翻译为 {title, detail?}；未知错误给出通用标题与可展开技术详情。 */
export function connectorErrorText(e: unknown): FriendlyError {
  if (e instanceof ApiError) {
    const byCode = CONNECTOR_CODE_TITLES[e.code]
    if (byCode) return { title: byCode }
    if (/base_url is required/.test(e.message)) return { title: '请填写服务地址。' }
    if (/spec is required/.test(e.message)) return { title: '请提供接口文档。' }
    return { title: CONNECTORS.errorGeneric, detail: `${e.code}: ${e.message}` }
  }
  return friendlyError(e)
}

/** 连接卡上的一行权限摘要；两项皆空返回 null。 */
export function permissionSummary(loginNames: string[], approvalNames: string[]): string | null {
  const parts: string[] = []
  if (loginNames.length > 0) parts.push(`${loginNames.length} 个工具需本人登录`)
  if (approvalNames.length > 0) parts.push(`${approvalNames.length} 个需审批`)
  return parts.length > 0 ? parts.join(' · ') : null
}
```

- [ ] **步骤 4：运行测试验证通过**

```powershell
npm run test -- src/strings.connector.test.ts
npx tsc --noEmit
```

预期：PASS，类型 0 错误。

- [ ] **步骤 5：Commit**

```powershell
git add src/strings.ts src/strings.connector.test.ts
git commit --trailer "Co-authored-by: Cursor <cursoragent@cursor.com>" -m "feat(web): 连接器人话文案与错误码中文化"
```
---

### 任务 3：连接器表单校验与权限纯函数

**文件：**
- 创建：`web/chat/src/pages/connectorForms/types.ts`
- 创建：`web/chat/src/pages/connectorForms/validate.ts`
- 创建：`web/chat/src/pages/connectorForms/permissions.ts`
- 测试：`web/chat/src/pages/connectorForms/validate.test.ts`、`permissions.test.ts`

- [ ] **步骤 1：编写失败测试 —— `validate.test.ts`**

```ts
import { describe, expect, it } from 'vitest'
import { validateConnection, CONNECTOR_ID_RE } from './validate'

describe('CONNECTOR_ID_RE', () => {
  it('accepts valid ids', () => {
    expect(CONNECTOR_ID_RE.test('ticket-api')).toBe(true)
    expect(CONNECTOR_ID_RE.test('a')).toBe(true)
    expect(CONNECTOR_ID_RE.test('crm_2')).toBe(true)
  })
  it('rejects invalid ids', () => {
    expect(CONNECTOR_ID_RE.test('Ticket')).toBe(false)
    expect(CONNECTOR_ID_RE.test('2start')).toBe(false)
    expect(CONNECTOR_ID_RE.test('has space')).toBe(false)
    expect(CONNECTOR_ID_RE.test('')).toBe(false)
  })
})

describe('validateConnection (plugin: no spec)', () => {
  it('requires id and base url', () => {
    const r = validateConnection({ kind: 'plugin', id: '  ', baseUrl: '', hasSpec: false })
    expect(r.ok).toBe(false)
    if (!r.ok) {
      expect(r.fieldErrors.id).toBeTruthy()
      expect(r.fieldErrors.baseUrl).toBeTruthy()
    }
  })
  it('accepts a valid plugin form', () => {
    const r = validateConnection({ kind: 'plugin', id: 'p1', baseUrl: ' http://127.0.0.1:19090 ', hasSpec: false })
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.baseUrl).toBe('http://127.0.0.1:19090')
  })
  it('rejects non-http base url', () => {
    const r = validateConnection({ kind: 'plugin', id: 'p1', baseUrl: 'ftp://x', hasSpec: false })
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.fieldErrors.baseUrl).toBeTruthy()
  })
})

describe('validateConnection (openapi: needs spec on create)', () => {
  it('requires a spec on create', () => {
    const r = validateConnection({ kind: 'openapi', id: 'o1', baseUrl: 'https://x', hasSpec: false })
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.fieldErrors.spec).toBeTruthy()
  })
  it('accepts when a spec file or url is present', () => {
    const r = validateConnection({ kind: 'openapi', id: 'o1', baseUrl: 'https://x', hasSpec: true })
    expect(r.ok).toBe(true)
  })
  it('editing an existing connector allows omitting a new spec', () => {
    const r = validateConnection({ kind: 'openapi', id: 'o1', baseUrl: 'https://x', hasSpec: false, editing: true })
    expect(r.ok).toBe(true)
  })
})
```

- [ ] **步骤 2：编写失败测试 —— `permissions.test.ts`**

```ts
import { describe, expect, it } from 'vitest'
import { emptySelection, selectionFromLists, toggleTool, toNameLists } from './permissions'

const tools = [
  { name: 'login' },
  { name: 'list_tickets' },
  { name: 'create_ticket' },
]

describe('selection helpers', () => {
  it('builds selection from existing name lists', () => {
    const sel = selectionFromLists(tools, ['login'], ['create_ticket'])
    expect(sel.login.login).toBe(true)
    expect(sel.login.approval).toBe(false)
    expect(sel.create_ticket.approval).toBe(true)
  })
  it('toggles a flag immutably', () => {
    const sel = emptySelection(tools)
    const next = toggleTool(sel, 'login', 'login')
    expect(sel.login.login).toBe(false)
    expect(next.login.login).toBe(true)
  })
  it('converts back to sorted name lists', () => {
    let sel = selectionFromLists(tools, ['create_ticket', 'login'], ['login'])
    sel = toggleTool(sel, 'list_tickets', 'approval')
    const { loginNames, approvalNames } = toNameLists(sel)
    expect(loginNames).toEqual(['create_ticket', 'login'])
    expect(approvalNames).toEqual(['list_tickets', 'login'])
  })
})
```

- [ ] **步骤 3：运行确认失败**

```powershell
cd web/chat
npm run test -- src/pages/connectorForms
```

预期：FAIL（模块不存在）。

- [ ] **步骤 4：实现 `types.ts`**

```ts
export type ConnectorKind = 'openapi' | 'plugin'

export interface ConnectionFormValues {
  kind: ConnectorKind
  id: string
  baseUrl: string
  /** 业务系统：本次是否提供了新文档（文件内容或非空 URL）。 */
  hasSpec: boolean
  /** 编辑既有连接时为 true，允许不带新文档保存。 */
  editing?: boolean
}

export type FieldErrors = Partial<Record<'id' | 'baseUrl' | 'spec', string>>

export type PermissionFlag = 'login' | 'approval'

export interface ToolPermission {
  login: boolean
  approval: boolean
}

/** key 为工具名。 */
export type PermissionSelection = Record<string, ToolPermission>
```

- [ ] **步骤 5：实现 `validate.ts`**

```ts
import { CONNECTORS } from '../../strings'
import type { ConnectionFormValues, FieldErrors } from './types'

export const CONNECTOR_ID_RE = /^[a-z][a-z0-9_-]{0,63}$/
const HTTP_URL_RE = /^https?:\/\/\S+$/i

export type ValidationResult =
  | { ok: true; id: string; baseUrl: string }
  | { ok: false; fieldErrors: FieldErrors }

export function validateConnection(v: ConnectionFormValues): ValidationResult {
  const fieldErrors: FieldErrors = {}
  const id = v.id.trim()
  const baseUrl = v.baseUrl.trim()

  if (!id) fieldErrors.id = CONNECTORS.errIdRequired
  else if (!CONNECTOR_ID_RE.test(id)) fieldErrors.id = CONNECTORS.errIdPattern

  if (!baseUrl) fieldErrors.baseUrl = CONNECTORS.errBaseUrlRequired
  else if (!HTTP_URL_RE.test(baseUrl)) fieldErrors.baseUrl = CONNECTORS.errBaseUrlHttp

  if (v.kind === 'openapi' && !v.hasSpec && !v.editing) {
    fieldErrors.spec = CONNECTORS.errSpecRequired
  }

  if (Object.keys(fieldErrors).length > 0) return { ok: false, fieldErrors }
  return { ok: true, id, baseUrl }
}
```

- [ ] **步骤 6：实现 `permissions.ts`**

```ts
import type { PermissionFlag, PermissionSelection } from './types'

interface NamedTool {
  name: string
}

export function emptySelection(tools: NamedTool[]): PermissionSelection {
  const sel: PermissionSelection = {}
  for (const t of tools) sel[t.name] = { login: false, approval: false }
  return sel
}

export function selectionFromLists(
  tools: NamedTool[],
  loginNames: readonly string[],
  approvalNames: readonly string[],
): PermissionSelection {
  const login = new Set(loginNames)
  const approval = new Set(approvalNames)
  const sel: PermissionSelection = {}
  for (const t of tools) {
    sel[t.name] = { login: login.has(t.name), approval: approval.has(t.name) }
  }
  return sel
}

export function toggleTool(
  sel: PermissionSelection,
  name: string,
  flag: PermissionFlag,
): PermissionSelection {
  const cur = sel[name] ?? { login: false, approval: false }
  return {
    ...sel,
    [name]: { ...cur, [flag]: !cur[flag] },
  }
}

export function toNameLists(sel: PermissionSelection): {
  loginNames: string[]
  approvalNames: string[]
} {
  const loginNames: string[] = []
  const approvalNames: string[] = []
  for (const [name, p] of Object.entries(sel)) {
    if (p.login) loginNames.push(name)
    if (p.approval) approvalNames.push(name)
  }
  loginNames.sort()
  approvalNames.sort()
  return { loginNames, approvalNames }
}
```

- [ ] **步骤 7：运行验证通过并 Commit**

```powershell
npm run test -- src/pages/connectorForms
npx tsc --noEmit
cd ../..
git add web/chat/src/pages/connectorForms
git commit --trailer "Co-authored-by: Cursor <cursoragent@cursor.com>" -m "feat(web): 连接器表单校验与工具权限纯函数"
```

预期：PASS。
---

### 任务 4：连接器列表外壳 `ConnectorShell`

**文件：**
- 创建：`web/chat/src/components/settings/ConnectorShell.tsx`
- 测试：`web/chat/src/components/settings/ConnectorShell.test.tsx`
- 修改：`web/chat/src/styles/components.css`（少量样式）

- [ ] **步骤 1：编写失败测试**

新建 `ConnectorShell.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ConnectorShell, type ConnectorRowData } from './ConnectorShell'

let host: HTMLDivElement
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host) })
afterEach(() => { host.remove() })

const flush = async () => {
  await act(async () => { await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0)) })
}
const btn = (txt: string) => [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(txt))!

async function render(props: Parameters<typeof ConnectorShell>[0]) {
  await act(async () => {
    createRoot(host).render(
      <MemoryRouter><ConnectorShell {...props} /></MemoryRouter>,
    )
    await new Promise((r) => setTimeout(r, 0))
  })
}

const rows: ConnectorRowData[] = [
  { id: 'ticket-api', baseUrl: 'https://api.example.com', toolCount: 3, loginNames: ['login'], approvalNames: ['create_ticket'] },
]

describe('ConnectorShell', () => {
  it('renders openapi header, add button and a card with permission summary', async () => {
    await render({ kind: 'openapi', rows, loading: false, loadError: null, onCreate: vi.fn(), onEdit: vi.fn(), onDelete: vi.fn() })
    expect(host.textContent).toContain('业务系统')
    expect(host.textContent).toContain('ticket-api')
    expect(host.textContent).toContain('3 个工具')
    expect(host.textContent).toContain('1 个工具需本人登录 · 1 个需审批')
    expect(btn('接入业务系统')).toBeTruthy()
  })

  it('shows friendly empty state', async () => {
    await render({ kind: 'plugin', rows: [], loading: false, loadError: null, onCreate: vi.fn(), onEdit: vi.fn(), onDelete: vi.fn() })
    expect(host.textContent).toContain('还没有接入插件服务')
    expect(host.textContent).toContain('接入插件服务')
  })

  it('opens row menu and confirms delete', async () => {
    const onDelete = vi.fn().mockResolvedValue(undefined)
    await render({ kind: 'openapi', rows, loading: false, loadError: null, onCreate: vi.fn(), onEdit: vi.fn(), onDelete })
    await act(async () => {
      ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await act(async () => {
      ;[...host.querySelectorAll('.dropdown-item')].find((i) => i.textContent!.includes('删除'))!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(host.textContent).toContain('删除这个连接？')
    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLElement).click()
      await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0))
    })
    expect(onDelete).toHaveBeenCalledWith('ticket-api')
  })
})
```

- [ ] **步骤 2：运行确认失败**

```powershell
cd web/chat
npm run test -- src/components/settings/ConnectorShell
```

预期：FAIL（模块不存在）。

- [ ] **步骤 3：实现 `ConnectorShell.tsx`**

```tsx
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Badge, Button, Card, ConfirmDialog, DropdownMenu, EmptyState, PageHeader } from '../ui'
import { CONNECTORS, permissionSummary } from '../../strings'
import type { ConnectorKind } from '../../pages/connectorForms/types'

export interface ConnectorRowData {
  id: string
  baseUrl?: string
  toolCount: number
  loginNames: string[]
  approvalNames: string[]
}

export interface ConnectorShellProps {
  kind: ConnectorKind
  rows: ConnectorRowData[]
  loading: boolean
  loadError: string | null
  onCreate: () => void
  onEdit: (id: string) => void
  onDelete: (id: string) => Promise<void> | void
}

export function ConnectorShell({ kind, rows, loading, loadError, onCreate, onEdit, onDelete }: ConnectorShellProps) {
  const [pendingDelete, setPendingDelete] = useState<string | null>(null)
  const [deleting, setDeleting] = useState(false)

  const isOpenapi = kind === 'openapi'
  const title = isOpenapi ? CONNECTORS.openapiTitle : CONNECTORS.pluginTitle
  const description = isOpenapi ? CONNECTORS.openapiDesc : CONNECTORS.pluginDesc
  const addLabel = isOpenapi ? CONNECTORS.addOpenapi : CONNECTORS.addPlugin
  const emptyTitle = isOpenapi ? CONNECTORS.openapiEmptyTitle : CONNECTORS.pluginEmptyTitle
  const emptyDesc = isOpenapi ? CONNECTORS.openapiEmptyDesc : CONNECTORS.pluginEmptyDesc

  const confirmDelete = async () => {
    if (!pendingDelete) return
    setDeleting(true)
    try {
      await onDelete(pendingDelete)
      setPendingDelete(null)
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className="settings-panel">
      <PageHeader
        title={title}
        description={description}
        actions={<Button variant="primary" size="sm" onClick={onCreate}>{addLabel}</Button>}
      />

      {loadError && <p className="ui-inline-error" role="alert">{loadError}</p>}
      {loading && rows.length === 0 && <p className="settings-muted">加载中…</p>}

      {!loading && rows.length === 0 && !loadError && (
        <EmptyState title={emptyTitle} description={emptyDesc}
          action={<Button variant="primary" onClick={onCreate}>{addLabel}</Button>} />
      )}

      {rows.length > 0 && (
        <div className="connector-list">
          {rows.map((row) => {
            const summary = permissionSummary(row.loginNames, row.approvalNames)
            return (
              <Card key={row.id} className="connector-card"
                title={row.id}
                description={row.baseUrl || '—'}
                trailing={<Badge tone="info">{CONNECTORS.toolCount(row.toolCount)}</Badge>}>
                {summary && <p className="connector-perm">{summary}</p>}
                <div className="connector-card-actions">
                  <Link to="/settings/tools" className="btn ghost sm">{CONNECTORS.toolsLink}</Link>
                  <DropdownMenu
                    triggerLabel={`${row.id} 操作`}
                    items={[
                      { id: 'edit', label: CONNECTORS.menuEdit, onSelect: () => onEdit(row.id) },
                      { id: 'delete', label: CONNECTORS.menuDelete, destructive: true, onSelect: () => setPendingDelete(row.id) },
                    ]}
                  />
                </div>
              </Card>
            )
          })}
        </div>
      )}

      <ConfirmDialog
        open={pendingDelete !== null}
        danger
        title={CONNECTORS.deleteTitle}
        body={CONNECTORS.deleteBody}
        confirmText={CONNECTORS.deleteOk}
        busy={deleting}
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => void confirmDelete()}
      />
    </div>
  )
}
```

- [ ] **步骤 4：追加样式**

在 `web/chat/src/styles/components.css` 末尾追加：

```css
.connector-list { display: flex; flex-direction: column; gap: var(--space-3); margin-top: var(--space-4); }
.connector-card-actions { display: flex; align-items: center; justify-content: space-between; margin-top: var(--space-2); }
.connector-perm { margin: var(--space-1) 0 0; color: var(--text-muted); font-size: 0.875rem; }
```

（若 `--space-1/2/3/4` 中某个未定义，改用已存在的相邻间距 token；以 `tokens.css` 实际定义为准。）

- [ ] **步骤 5：运行验证通过并 Commit**

```powershell
npm run test -- src/components/settings/ConnectorShell
npx tsc --noEmit
cd ../..
git add web/chat/src/components/settings web/chat/src/styles/components.css
git commit --trailer "Co-authored-by: Cursor <cursoragent@cursor.com>" -m "feat(web): 连接器列表外壳 ConnectorShell"
```

预期：PASS。
---

### 任务 5：两步编辑器 `ConnectorEditorModal`

**文件：**
- 创建：`web/chat/src/components/settings/ConnectorEditorModal.tsx`
- 测试：`web/chat/src/components/settings/ConnectorEditorModal.test.tsx`
- 修改：`web/chat/src/styles/components.css`

该组件只负责展示与本地状态；所有网络调用由父页面通过 `onSaveInfo` / `onSavePermissions` 注入，便于测试。

**Props 契约：**

```ts
interface ConnectorEditorModalProps {
  kind: ConnectorKind            // 'openapi' | 'plugin'
  open: boolean
  editing: boolean               // 编辑既有连接
  initial: {
    id: string
    baseUrl: string
    tools: { name: string }[]
    loginNames: string[]
    approvalNames: string[]
  }
  onClose: () => void
  // 第一步保存：返回该连接最新的工具列表（putConnector 响应 .tools）。
  onSaveInfo: (input: {
    id: string
    baseUrl: string
    spec?: { content?: string; url?: string }
    importFormat: ImportFormat
  }) => Promise<{ name: string }[]>
  // 第二步保存：提交服务地址与两个工具名列表（二次 PUT 必须带 base_url，
  // openapi 不传文档时后端复用已存 spec）。
  onSavePermissions: (
    id: string,
    baseUrl: string,
    loginNames: string[],
    approvalNames: string[],
  ) => Promise<void>
}
```

- [ ] **步骤 1：编写失败测试**

新建 `ConnectorEditorModal.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ConnectorEditorModal } from './ConnectorEditorModal'

let host: HTMLDivElement
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host) })
afterEach(() => { host.remove() })

const flush = async (n = 3) => { await act(async () => { for (let i = 0; i < n; i++) await new Promise((r) => setTimeout(r, 0)) }) }
const setValue = (el: Element, value: string) => act(async () => {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
  setter.call(el, value); el.dispatchEvent(new Event('input', { bubbles: true })); await new Promise((r) => setTimeout(r, 0))
})
const btn = (txt: string) => [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(txt))!

const emptyInitial = { id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [] }

async function render(props: Partial<Parameters<typeof ConnectorEditorModal>[0]> = {}) {
  await act(async () => {
    createRoot(host).render(
      <ConnectorEditorModal
        kind="openapi" open editing={false} initial={emptyInitial}
        onClose={vi.fn()}
        onSaveInfo={vi.fn(async () => [{ name: 'login' }, { name: 'list_tickets' }])}
        onSavePermissions={vi.fn(async () => {})}
        {...props}
      />,
    )
    await new Promise((r) => setTimeout(r, 0))
  })
}

describe('ConnectorEditorModal step 1', () => {
  it('validates required fields before saving', async () => {
    const onSaveInfo = vi.fn(async () => [])
    await render({ onSaveInfo })
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    expect(host.textContent).toContain('请填写连接编号')
    expect(host.textContent).toContain('请填写服务地址')
    expect(onSaveInfo).not.toHaveBeenCalled()
  })

  it('openapi create requires a spec', async () => {
    const onSaveInfo = vi.fn(async () => [])
    await render({ onSaveInfo })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'o1')
    await setValue(inputs[1], 'https://api.example.com')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    expect(host.textContent).toContain('请上传接口文档或填写文档链接')
    expect(onSaveInfo).not.toHaveBeenCalled()
  })

  it('plugin create saves with id + base url and moves to step 2', async () => {
    const onSaveInfo = vi.fn(async () => [{ name: 'ping' }])
    await render({ kind: 'plugin', onSaveInfo })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'p1')
    await setValue(inputs[1], 'http://127.0.0.1:19090')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    expect(onSaveInfo).toHaveBeenCalledWith(expect.objectContaining({ id: 'p1' }))
    expect(host.textContent).toContain('工具权限')
    expect(host.textContent).toContain('ping')
  })
})

describe('ConnectorEditorModal step 2', () => {
  const editInitial = {
    id: 'o1', baseUrl: 'https://x',
    tools: [{ name: 'login' }, { name: 'create_ticket' }],
    loginNames: ['login'], approvalNames: ['create_ticket'],
  }

  it('opens directly at step 1 but can reach step 2 with saved tools and echoes checkboxes', async () => {
    await render({ editing: true, initial: editInitial })
    // 编辑既有连接：通过「下一步：设置工具权限」进入第二步（无需再次保存）
    await act(async () => { btn('设置工具权限').click(); await new Promise((r) => setTimeout(r, 0)) })
    const boxes = [...host.querySelectorAll('input[type="checkbox"]')] as HTMLInputElement[]
    const loginBox = boxes.find((b) => b.dataset.tool === 'login' && b.dataset.flag === 'login')!
    const approvalBox = boxes.find((b) => b.dataset.tool === 'create_ticket' && b.dataset.flag === 'approval')!
    expect(loginBox.checked).toBe(true)
    expect(approvalBox.checked).toBe(true)
  })

  it('saves selected permission lists', async () => {
    const onSavePermissions = vi.fn(async () => {})
    await render({ editing: true, initial: editInitial, onSavePermissions })
    await act(async () => { btn('设置工具权限').click(); await new Promise((r) => setTimeout(r, 0)) })
    await act(async () => { btn('完成').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    expect(onSavePermissions).toHaveBeenCalledWith('o1', 'https://x', ['login'], ['create_ticket'])
  })
})
```

注意：实现时给每个权限复选框加 `data-tool=<name>` 与 `data-flag="login|approval"`，供测试定位（替代靠顺序猜测）。
- [ ] **步骤 2：运行确认失败**

```powershell
cd web/chat
npm run test -- src/components/settings/ConnectorEditorModal
```

预期：FAIL（模块不存在）。

- [ ] **步骤 3：实现 `ConnectorEditorModal.tsx`**

```tsx
import { type ChangeEvent, useEffect, useState } from 'react'
import { Button, Field, Input, Modal, Select } from '../ui'
import { CONNECTORS } from '../../strings'
import { validateConnection } from '../../pages/connectorForms/validate'
import {
  emptySelection,
  selectionFromLists,
  toNameLists,
  toggleTool,
} from '../../pages/connectorForms/permissions'
import type { ConnectorKind, FieldErrors, PermissionSelection } from '../../pages/connectorForms/types'
import type { ImportFormat } from '../../api'

export interface ConnectorEditorInitial {
  id: string
  baseUrl: string
  tools: { name: string }[]
  loginNames: string[]
  approvalNames: string[]
}

export interface ConnectorEditorModalProps {
  kind: ConnectorKind
  open: boolean
  editing: boolean
  initial: ConnectorEditorInitial
  onClose: () => void
  formatError: (e: unknown) => string
  onSaveInfo: (input: {
    id: string
    baseUrl: string
    spec?: { content?: string; url?: string }
    importFormat: ImportFormat
  }) => Promise<{ name: string }[]>
  onSavePermissions: (id: string, loginNames: string[], approvalNames: string[]) => Promise<void>
}

const EMPTY_INITIAL: ConnectorEditorInitial = {
  id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [],
}

export function ConnectorEditorModal(props: ConnectorEditorModalProps) {
  const { kind, open, editing, initial = EMPTY_INITIAL } = props
  const [step, setStep] = useState<1 | 2>(1)
  const [id, setId] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [importFormat, setImportFormat] = useState<ImportFormat>('auto')
  const [specContent, setSpecContent] = useState<string | null>(null)
  const [specFileName, setSpecFileName] = useState<string | null>(null)
  const [specUrl, setSpecUrl] = useState('')
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})
  const [formError, setFormError] = useState<string | null>(null)
  const [tools, setTools] = useState<{ name: string }[]>([])
  const [selection, setSelection] = useState<PermissionSelection>({})
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!open) return
    setStep(1)
    setId(initial.id)
    setBaseUrl(initial.baseUrl)
    setImportFormat('auto')
    setSpecContent(null)
    setSpecFileName(null)
    setSpecUrl('')
    setFieldErrors({})
    setFormError(null)
    setSaving(false)
    setTools(initial.tools)
    setSelection(selectionFromLists(initial.tools, initial.loginNames, initial.approvalNames))
  }, [open, initial])

  if (!open) return null

  const isOpenapi = kind === 'openapi'
  const title = editing
    ? (isOpenapi ? CONNECTORS.editOpenapi : CONNECTORS.editPlugin)
    : (isOpenapi ? CONNECTORS.addOpenapi : CONNECTORS.addPlugin)

  const onSpecFile = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      setSpecContent(typeof reader.result === 'string' ? reader.result : '')
      setSpecFileName(file.name)
      setSpecUrl('')
      setFieldErrors((prev) => ({ ...prev, spec: undefined }))
    }
    reader.readAsText(file)
  }

  const saveInfo = async () => {
    const hasSpec = specContent != null || specUrl.trim() !== ''
    const result = validateConnection({ kind, id, baseUrl, hasSpec, editing })
    if (!result.ok) {
      setFieldErrors(result.fieldErrors)
      return
    }
    setFieldErrors({})
    setFormError(null)
    setSaving(true)
    try {
      const discovered = await props.onSaveInfo({
        id: result.id,
        baseUrl: result.baseUrl,
        spec: isOpenapi && hasSpec
          ? { content: specContent ?? undefined, url: specContent == null ? specUrl.trim() : undefined }
          : undefined,
        importFormat,
      })
      const nextTools = discovered.length > 0 ? discovered : initial.tools
      setTools(nextTools)
      setSelection(
        editing
          ? selectionFromLists(nextTools, initial.loginNames, initial.approvalNames)
          : emptySelection(nextTools),
      )
      setStep(2)
    } catch (e) {
      setFormError(props.formatError(e))
    } finally {
      setSaving(false)
    }
  }

  const gotoPermissions = () => {
    setTools(initial.tools)
    setSelection(selectionFromLists(initial.tools, initial.loginNames, initial.approvalNames))
    setStep(2)
  }

  const finish = async () => {
    setSaving(true)
    try {
      const { loginNames, approvalNames } = toNameLists(selection)
      await props.onSavePermissions(id.trim(), baseUrl.trim(), loginNames, approvalNames)
      props.onClose()
    } catch (e) {
      setFormError(props.formatError(e))
      setSaving(false)
    }
  }

  return (
    <Modal
      open={open}
      title={title}
      onClose={saving ? undefined : props.onClose}
      footer={
        step === 1 ? (
          <>
            <Button variant="secondary" onClick={props.onClose} disabled={saving}>{CONNECTORS.cancel}</Button>
            {editing && initial.tools.length > 0 && (
              <Button variant="secondary" onClick={gotoPermissions} disabled={saving}>
                {CONNECTORS.nextToPermissions}
              </Button>
            )}
            <Button variant="primary" onClick={() => void saveInfo()} disabled={saving}>
              {saving ? CONNECTORS.saving : CONNECTORS.save}
            </Button>
          </>
        ) : (
          <>
            <Button variant="secondary" onClick={() => setStep(1)} disabled={saving}>{CONNECTORS.back}</Button>
            {!editing && (
              <Button variant="secondary" onClick={props.onClose} disabled={saving}>{CONNECTORS.skip}</Button>
            )}
            <Button variant="primary" onClick={() => void finish()} disabled={saving}>
              {saving ? CONNECTORS.saving : CONNECTORS.finish}
            </Button>
          </>
        )
      }
    >
      {formError && <p className="ui-inline-error" role="alert">{formError}</p>}
      {step === 1 ? (
        <div className="connector-form">
          <Field label={CONNECTORS.fieldId} hint={CONNECTORS.fieldIdHint} required error={fieldErrors.id}>
            <Input value={id} disabled={editing || saving} placeholder="ticket-api"
              onChange={(e) => { setId(e.target.value); setFieldErrors((p) => ({ ...p, id: undefined })) }} />
          </Field>
          <Field label={CONNECTORS.fieldBaseUrl} required error={fieldErrors.baseUrl}>
            <Input value={baseUrl} disabled={saving}
              placeholder={isOpenapi ? 'https://api.example.com' : 'http://127.0.0.1:19090'}
              onChange={(e) => { setBaseUrl(e.target.value); setFieldErrors((p) => ({ ...p, baseUrl: undefined })) }} />
          </Field>
          {isOpenapi && (
            <>
              <Field label={CONNECTORS.fieldSpec} hint={CONNECTORS.fieldSpecHint} error={fieldErrors.spec}>
                <Input type="file" accept=".json,.yaml,.yml" disabled={saving} onChange={onSpecFile} />
              </Field>
              {specFileName && <p className="ui-field-hint">{CONNECTORS.specFileChosen(specFileName)}</p>}
              <Field label={CONNECTORS.fieldSpecUrl}>
                <Input value={specUrl} disabled={saving || specContent != null}
                  placeholder="https://api.example.com/openapi.json"
                  onChange={(e) => { setSpecUrl(e.target.value); setFieldErrors((p) => ({ ...p, spec: undefined })) }} />
              </Field>
              <Field label={CONNECTORS.fieldFormat}>
                <Select value={importFormat} disabled={saving}
                  onChange={(e) => setImportFormat(e.target.value as ImportFormat)}>
                  <option value="auto">{CONNECTORS.fmtAuto}</option>
                  <option value="openapi3">{CONNECTORS.fmtOpenapi3}</option>
                  <option value="swagger2">{CONNECTORS.fmtSwagger2}</option>
                  <option value="postman">{CONNECTORS.fmtPostman}</option>
                </Select>
              </Field>
            </>
          )}
        </div>
      ) : (
        <div className="connector-permissions">
          <p className="connector-perms-intro">{CONNECTORS.permsIntro}</p>
          {tools.length === 0 && <p className="settings-muted">暂无已识别工具。</p>}
          {tools.map((t) => (
            <div key={t.name} className="connector-perm-row">
              <span className="connector-perm-name">{t.name}</span>
              <label className="ui-checkbox-row">
                <input
                  type="checkbox"
                  data-tool={t.name}
                  data-flag="login"
                  checked={selection[t.name]?.login ?? false}
                  disabled={saving}
                  onChange={() => setSelection((s) => toggleTool(s, t.name, 'login'))}
                />
                <span>{CONNECTORS.permLogin}</span>
              </label>
              <label className="ui-checkbox-row">
                <input
                  type="checkbox"
                  data-tool={t.name}
                  data-flag="approval"
                  checked={selection[t.name]?.approval ?? false}
                  disabled={saving}
                  onChange={() => setSelection((s) => toggleTool(s, t.name, 'approval'))}
                />
                <span>{CONNECTORS.permApproval}</span>
              </label>
            </div>
          ))}
        </div>
      )}
    </Modal>
  )
}
```

修正：第一步 footer 的取消按钮必须用 `CONNECTORS.cancel`（已在任务 2 文案中定义），不要写条件表达式。

- [ ] **步骤 4：追加样式**

在 `web/chat/src/styles/components.css` 末尾追加：

```css
.connector-form { display: flex; flex-direction: column; gap: var(--space-3); }
.connector-perms-intro { color: var(--text-muted); font-size: 0.875rem; margin: 0 0 var(--space-3); }
.connector-perm-row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-4); padding: var(--space-2) 0; border-bottom: 1px solid var(--border); }
.connector-perm-name { font-family: var(--font-mono, monospace); font-size: 0.875rem; min-width: 8rem; }
```

（token 名以 `tokens.css` 实际定义为准；缺失时改用相邻已存在 token。）

- [ ] **步骤 5：运行验证通过并 Commit**

```powershell
npm run test -- src/components/settings/ConnectorEditorModal
npx tsc --noEmit
cd ../..
git add web/chat/src/components/settings web/chat/src/styles/components.css web/chat/src/strings.ts
git commit --trailer "Co-authored-by: Cursor <cursoragent@cursor.com>" -m "feat(web): 两步连接器编辑器（连接信息 + 工具权限）"
```

预期：PASS。
---

### 任务 6：插件页瘦身为薄页面

**文件：**
- 改写：`web/chat/src/pages/PluginSettings.tsx`
- 改写：`web/chat/src/pages/PluginSettings.test.ts`

数据加载沿用现状：`listTools()` 筛 `source==='plugin'` 得到连接器 id，再逐个 `getConnector`。删除旧的鉴权 / 抽屉 / 手填工具名逻辑。

- [ ] **步骤 1：改写页面测试（先红）**

`PluginSettings.test.ts` 整体替换为：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PluginSettings } from './PluginSettings'

function json(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' }, ...init })
}
let host: HTMLDivElement
let fetchMock: ReturnType<typeof vi.fn>
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host); fetchMock = vi.fn(); vi.stubGlobal('fetch', fetchMock) })
afterEach(() => { host.remove(); vi.unstubAllGlobals() })
const flush = async (n = 3) => { await act(async () => { for (let i = 0; i < n; i++) await new Promise((r) => setTimeout(r, 0)) }) }
const btn = (txt: string) => [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(txt))!
const setValue = (el: Element, v: string) => act(async () => {
  const s = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
  s.call(el, v); el.dispatchEvent(new Event('input', { bubbles: true })); await new Promise((r) => setTimeout(r, 0))
})

const pluginConnector = {
  id: 'legacy', type: 'http', base_url: 'http://127.0.0.1:19090',
  require_login: ['ping'], require_approval: [],
  tools: [{ name: 'ping' }],
}

async function renderPlugin() {
  fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
    const u = String(url)
    if (!init?.method && u.endsWith('/v0/tools')) return json({ tools: [{ name: 'ping', connector_id: 'legacy', source: 'plugin' }] })
    if (!init?.method && u.includes('/v0/connectors/legacy')) return json(pluginConnector)
    if (init?.method === 'PUT') return json({ ...pluginConnector, tools: [{ name: 'ping' }], require_login: [], require_approval: [] })
    return json({})
  })
  await act(async () => { createRoot(host).render(<MemoryRouter><PluginSettings /></MemoryRouter>); await new Promise((r) => setTimeout(r, 0)) })
  await flush()
}

describe('PluginSettings page', () => {
  it('renders humanized list card with permission summary', async () => {
    await renderPlugin()
    expect(host.textContent).toContain('插件服务')
    expect(host.textContent).toContain('legacy')
    expect(host.textContent).toContain('1 个工具需本人登录')
  })

  it('creates a plugin with id+base url then saves permissions in a second PUT', async () => {
    await renderPlugin()
    await act(async () => { btn('接入插件服务').click(); await new Promise((r) => setTimeout(r, 0)) })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'p1')
    await setValue(inputs[1], 'http://127.0.0.1:19090')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    // 进入第二步
    expect(host.textContent).toContain('工具权限')
    await act(async () => {
      const box = host.querySelector('input[data-flag="login"]') as HTMLInputElement
      box.click(); await new Promise((r) => setTimeout(r, 0))
    })
    await act(async () => { btn('完成').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    const puts = fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')
    expect(puts).toHaveLength(2)
    const first = JSON.parse((puts[0][1] as RequestInit).body as string)
    const second = JSON.parse((puts[1][1] as RequestInit).body as string)
    expect(first).toMatchObject({ type: 'http', base_url: 'http://127.0.0.1:19090' })
    expect(first.auth).toBeUndefined()
    expect(second.require_login).toEqual(['ping'])
  })
})
```

- [ ] **步骤 2：运行确认失败**

```powershell
cd web/chat
npm run test -- src/pages/PluginSettings
```

预期：FAIL（页面尚未导出新结构 / 仍带鉴权）。

- [ ] **步骤 3：改写 `PluginSettings.tsx`**

整文件替换为：

```tsx
import { useCallback, useEffect, useState } from 'react'
import {
  deleteConnector,
  getConnector,
  listTools,
  putConnector,
  type ConnectorInfo,
  type ToolInfo,
} from '../api'
import { ToastRegion, useToast } from '../components/ui'
import { ConnectorShell, type ConnectorRowData } from '../components/settings/ConnectorShell'
import { ConnectorEditorModal, type ConnectorEditorInitial } from '../components/settings/ConnectorEditorModal'
import { CONNECTORS, connectorErrorText } from '../strings'

export function pluginConnectorIds(tools: ToolInfo[]): string[] {
  const seen = new Set<string>()
  const ordered: string[] = []
  for (const t of tools) {
    if (t.source !== 'plugin' || !t.connector_id || seen.has(t.connector_id)) continue
    seen.add(t.connector_id)
    ordered.push(t.connector_id)
  }
  return ordered
}

function toRow(info: ConnectorInfo, fallbackCount: number): ConnectorRowData {
  return {
    id: info.id,
    baseUrl: info.base_url,
    toolCount: info.tools?.length ?? fallbackCount,
    loginNames: info.require_login ?? [],
    approvalNames: info.require_approval ?? [],
  }
}

export function PluginSettings() {
  const { toasts, push, dismiss } = useToast()
  const [rows, setRows] = useState<ConnectorRowData[]>([])
  const [connectors, setConnectors] = useState<ConnectorInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [editor, setEditor] = useState<{ open: boolean; editing: boolean; initial: ConnectorEditorInitial }>({
    open: false, editing: false, initial: { id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [] },
  })

  const load = useCallback(async () => {
    try {
      const tools = await listTools()
      const countById = new Map<string, number>()
      for (const t of tools) {
        if (t.source === 'plugin' && t.connector_id) countById.set(t.connector_id, (countById.get(t.connector_id) ?? 0) + 1)
      }
      const infos = await Promise.all(pluginConnectorIds(tools).map((id) => getConnector(id)))
      setConnectors(infos)
      setRows(infos.map((c) => toRow(c, countById.get(c.id) ?? 0)))
      setLoadError(null)
    } catch (e) {
      setLoadError(connectorErrorText(e).title)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void load() }, [load])

  const openCreate = () => setEditor({ open: true, editing: false, initial: { id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [] } })
  const openEdit = (id: string) => {
    const c = connectors.find((x) => x.id === id)
    if (!c) return
    setEditor({
      open: true, editing: true,
      initial: {
        id: c.id, baseUrl: c.base_url ?? '',
        tools: (c.tools ?? []).map((t) => ({ name: t.name })),
        loginNames: c.require_login ?? [], approvalNames: c.require_approval ?? [],
      },
    })
  }

  const handleDelete = async (id: string) => {
    await deleteConnector(id)
    push({ tone: 'success', title: `${CONNECTORS.deleted} ${id}` })
    await load()
  }

  const handleSaveInfo = async (input: { id: string; baseUrl: string }) => {
    const c = await putConnector(input.id, { type: 'http', base_url: input.baseUrl })
    return (c.tools ?? []).map((t) => ({ name: t.name }))
  }
  const handleSavePermissions = async (id: string, baseUrl: string, loginNames: string[], approvalNames: string[]) => {
    await putConnector(id, { type: 'http', base_url: baseUrl, require_login: loginNames, require_approval: approvalNames })
    push({ tone: 'success', title: `${CONNECTORS.saved} ${id}` })
    await load()
  }

  return (
    <>
      <ConnectorShell kind="plugin" rows={rows} loading={loading} loadError={loadError}
        onCreate={openCreate} onEdit={openEdit} onDelete={handleDelete} />
      <ConnectorEditorModal kind="plugin" open={editor.open} editing={editor.editing} initial={editor.initial}
        onClose={() => setEditor((e) => ({ ...e, open: false }))}
        formatError={(e) => connectorErrorText(e).title}
        onSaveInfo={handleSaveInfo} onSavePermissions={handleSavePermissions} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />
    </>
  )
}
```

注意：`ConnectorEditorModal` 的 `onSaveInfo` 入参对插件只用到 `id/baseUrl`（`spec` 可忽略）；`onSavePermissions` 第二参为 `baseUrl`（见任务 5 的 Props，实现须以此为准）。

- [ ] **步骤 4：运行验证通过并 Commit**

```powershell
npm run test -- src/pages/PluginSettings src/components/settings
npx tsc --noEmit
cd ../..
git add web/chat/src/pages/PluginSettings.tsx web/chat/src/pages/PluginSettings.test.ts
git commit --trailer "Co-authored-by: Cursor <cursoragent@cursor.com>" -m "feat(web): 插件设置页迁移到连接器外壳并移除默认凭证编辑"
```

预期：PASS。若 `tsc` 报 `OpenApiSettings` 仍引用已删导出，属预期，任务 7 修复。
---

### 任务 7：业务系统页瘦身（文档上传 / URL）

**文件：**
- 改写：`web/chat/src/pages/OpenApiSettings.tsx`
- 改写：`web/chat/src/pages/OpenApiSettings.test.ts`

- [ ] **步骤 1：改写页面测试（先红）**

`OpenApiSettings.test.ts` 整体替换为：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { OpenApiSettings, openApiConnectorIds } from './OpenApiSettings'

function json(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' }, ...init })
}
let host: HTMLDivElement
let fetchMock: ReturnType<typeof vi.fn>
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host); fetchMock = vi.fn(); vi.stubGlobal('fetch', fetchMock) })
afterEach(() => { host.remove(); vi.unstubAllGlobals() })
const flush = async (n = 3) => { await act(async () => { for (let i = 0; i < n; i++) await new Promise((r) => setTimeout(r, 0)) }) }
const btn = (txt: string) => [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(txt))!
const setValue = (el: Element, v: string) => act(async () => {
  const s = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
  s.call(el, v); el.dispatchEvent(new Event('input', { bubbles: true })); await new Promise((r) => setTimeout(r, 0))
})

const conn = {
  id: 'ticket-api', type: 'openapi', base_url: 'https://api.example.com',
  require_login: ['me'], require_approval: ['create_ticket'],
  tools: [{ name: 'me' }, { name: 'create_ticket' }],
}

async function renderOpenApi() {
  fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
    const u = String(url)
    if (!init?.method && u.endsWith('/v0/tools'))
      return json({ tools: [{ name: 'me', connector_id: 'ticket-api', source: 'spec' }] })
    if (!init?.method && u.includes('/v0/connectors/ticket-api')) return json(conn)
    if (init?.method === 'PUT') return json({ ...conn, require_login: [], require_approval: [] })
    return json({})
  })
  await act(async () => { createRoot(host).render(<MemoryRouter><OpenApiSettings /></MemoryRouter>); await new Promise((r) => setTimeout(r, 0)) })
  await flush()
}

describe('openApiConnectorIds', () => {
  it('includes spec/extra sources only', () => {
    expect(openApiConnectorIds([
      { name: 'a', connector_id: 'o1', source: 'spec' },
      { name: 'b', connector_id: 'o2', source: 'extra' },
      { name: 'c', connector_id: 'p1', source: 'plugin' },
    ] as any)).toEqual(['o1', 'o2'])
  })
})

describe('OpenApiSettings page', () => {
  it('renders humanized card with counts', async () => {
    await renderOpenApi()
    expect(host.textContent).toContain('业务系统')
    expect(host.textContent).toContain('ticket-api')
    expect(host.textContent).toContain('1 个工具需本人登录 · 1 个需审批')
  })

  it('create without a spec is blocked; with url sends spec_url then permissions', async () => {
    await renderOpenApi()
    await act(async () => { btn('接入业务系统').click(); await new Promise((r) => setTimeout(r, 0)) })
    const textInputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(textInputs[0], 'o1')
    await setValue(textInputs[1], 'https://api.example.com')
    // 不提供文档：被拦截
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    expect(host.textContent).toContain('请上传接口文档或填写文档链接')
    // 文档链接输入是第三个文本框
    await setValue(textInputs[2], 'https://api.example.com/openapi.json')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    expect(host.textContent).toContain('工具权限')
    await act(async () => { btn('完成').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    const puts = fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')
    expect(puts.length).toBeGreaterThanOrEqual(2)
    const first = JSON.parse((puts[0][1] as RequestInit).body as string)
    expect(first).toMatchObject({ type: 'openapi', spec_url: 'https://api.example.com/openapi.json' })
    expect(first.auth).toBeUndefined()
  })
})
```

- [ ] **步骤 2：运行确认失败**

```powershell
cd web/chat
npm run test -- src/pages/OpenApiSettings
```

预期：FAIL（旧页面仍是抽屉 / 鉴权字段）。

- [ ] **步骤 3：改写 `OpenApiSettings.tsx`**

整文件替换为：

```tsx
import { useCallback, useEffect, useState } from 'react'
import {
  deleteConnector,
  getConnector,
  listTools,
  putConnector,
  type ConnectorInfo,
  type ToolInfo,
} from '../api'
import { ToastRegion, useToast } from '../components/ui'
import { ConnectorShell, type ConnectorRowData } from '../components/settings/ConnectorShell'
import { ConnectorEditorModal, type ConnectorEditorInitial } from '../components/settings/ConnectorEditorModal'
import { CONNECTORS, connectorErrorText } from '../strings'

export function openApiConnectorIds(tools: ToolInfo[]): string[] {
  const seen = new Set<string>()
  const ordered: string[] = []
  for (const t of tools) {
    if (t.source !== 'spec' && t.source !== 'extra') continue
    if (!t.connector_id || seen.has(t.connector_id)) continue
    seen.add(t.connector_id)
    ordered.push(t.connector_id)
  }
  return ordered
}

function toRow(info: ConnectorInfo, fallbackCount: number): ConnectorRowData {
  return {
    id: info.id,
    baseUrl: info.base_url,
    toolCount: info.tools?.length ?? fallbackCount,
    loginNames: info.require_login ?? [],
    approvalNames: info.require_approval ?? [],
  }
}

export function OpenApiSettings() {
  const { toasts, push, dismiss } = useToast()
  const [rows, setRows] = useState<ConnectorRowData[]>([])
  const [connectors, setConnectors] = useState<ConnectorInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const emptyInitial: ConnectorEditorInitial = { id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [] }
  const [editor, setEditor] = useState<{ open: boolean; editing: boolean; initial: ConnectorEditorInitial }>({
    open: false, editing: false, initial: emptyInitial,
  })

  const load = useCallback(async () => {
    try {
      const tools = await listTools()
      const countById = new Map<string, number>()
      for (const t of tools) {
        if ((t.source === 'spec' || t.source === 'extra') && t.connector_id)
          countById.set(t.connector_id, (countById.get(t.connector_id) ?? 0) + 1)
      }
      const infos = await Promise.all(openApiConnectorIds(tools).map((id) => getConnector(id)))
      setConnectors(infos)
      setRows(infos.map((c) => toRow(c, countById.get(c.id) ?? 0)))
      setLoadError(null)
    } catch (e) {
      setLoadError(connectorErrorText(e).title)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void load() }, [load])

  const openCreate = () => setEditor({ open: true, editing: false, initial: emptyInitial })
  const openEdit = (id: string) => {
    const c = connectors.find((x) => x.id === id)
    if (!c) return
    setEditor({
      open: true, editing: true,
      initial: {
        id: c.id, baseUrl: c.base_url ?? '',
        tools: (c.tools ?? []).map((t) => ({ name: t.name })),
        loginNames: c.require_login ?? [], approvalNames: c.require_approval ?? [],
      },
    })
  }

  const handleDelete = async (id: string) => {
    await deleteConnector(id)
    push({ tone: 'success', title: `${CONNECTORS.deleted} ${id}` })
    await load()
  }

  const handleSaveInfo = async (input: {
    id: string
    baseUrl: string
    spec?: { content?: string; url?: string }
    importFormat: import('../api').ImportFormat
  }) => {
    const c = await putConnector(input.id, {
      type: 'openapi',
      base_url: input.baseUrl,
      import_format: input.importFormat,
      spec_content: input.spec?.content,
      spec_url: input.spec?.url,
    })
    return (c.tools ?? []).map((t) => ({ name: t.name }))
  }

  const handleSavePermissions = async (
    id: string, baseUrl: string, loginNames: string[], approvalNames: string[],
  ) => {
    // 不传文档：后端对编辑/已存在连接器复用已保存 spec。
    await putConnector(id, {
      type: 'openapi',
      base_url: baseUrl,
      require_login: loginNames,
      require_approval: approvalNames,
    })
    push({ tone: 'success', title: `${CONNECTORS.saved} ${id}` })
    await load()
  }

  return (
    <>
      <ConnectorShell kind="openapi" rows={rows} loading={loading} loadError={loadError}
        onCreate={openCreate} onEdit={openEdit} onDelete={handleDelete} />
      <ConnectorEditorModal kind="openapi" open={editor.open} editing={editor.editing} initial={editor.initial}
        onClose={() => setEditor((e) => ({ ...e, open: false }))}
        formatError={(e) => connectorErrorText(e).title}
        onSaveInfo={handleSaveInfo} onSavePermissions={handleSavePermissions} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />
    </>
  )
}
```

- [ ] **步骤 4：运行验证通过并 Commit**

```powershell
npm run test -- src/pages/OpenApiSettings
npx tsc --noEmit
cd ../..
git add web/chat/src/pages/OpenApiSettings.tsx web/chat/src/pages/OpenApiSettings.test.ts
git commit --trailer "Co-authored-by: Cursor <cursoragent@cursor.com>" -m "feat(web): 业务系统设置页迁移到连接器外壳并移除默认凭证编辑"
```

预期：PASS、0 类型错误。
---

### 任务 8：清理旧凭证辅助函数并全量回归

**文件：**
- 修改：`web/chat/src/pages/captureForm.ts`
- 修改：`web/chat/src/pages/CaptureSettings.test.ts`

新两页已不再使用 `mergeAuthPreserveCapture` / `captureSummaryLabel` / `hasStoredCapture`（全局 grep 确认仅旧两页与 `CaptureSettings.test.ts` 引用；`ToolsSettings.tsx` 只用 `captureToDraft / buildCaptureFromDraft / mergeAuthWithCapture / connectorSupportsLoginCapture`，必须保留）。

- [ ] **步骤 1：删除死代码**

在 `captureForm.ts` 删除三个导出：`hasStoredCapture`、`captureSummaryLabel`、`mergeAuthPreserveCapture`。保留文件顶部类型与 `captureToDraft / formatPathLines / parsePathLines / buildCaptureFromDraft / mergeAuthWithCapture / connectorSupportsLoginCapture / EMPTY_CAPTURE_DRAFT / CaptureDraft`。

在 `CaptureSettings.test.ts`：

- 从 import 中移除 `captureSummaryLabel`、`mergeAuthPreserveCapture`；
- 删除 `describe('captureSummaryLabel', ...)` 与 `describe('mergeAuthPreserveCapture', ...)` 两个测试块；
- 保留 `captureToDraft` / `buildCaptureFromDraft` / `parsePathLines` / `mergeAuthWithCapture` 相关测试。

- [ ] **步骤 2：确认无残留引用**

```powershell
cd web/chat
findstr /S /N "mergeAuthPreserveCapture captureSummaryLabel hasStoredCapture authMode buildConnectorAuth PluginAuthMode" src\*.ts src\*.tsx
```

预期：只剩本计划/文档（`docs/` 不在 src 下）；`src` 下无业务代码命中（`McpSettings.tsx` 不引用这些）。若 `OpenApiSettings.tsx` 仍 import `./PluginSettings` 或 `./specImport` 的死符号，删除这些 import。

- [ ] **步骤 3：前端全量回归**

```powershell
npx tsc --noEmit
npm run test
npm run build
```

预期：0 类型错误；全部既有 + 新增测试通过；build 成功并刷新 `web/chat/dist`。

- [ ] **步骤 4：后端全量回归与格式化**

```powershell
cd ../..
$env:PATH = "C:\Users\Administrator\.local\go1.25.0\bin;" + $env:PATH
gofmt -l internal/api internal/connector
go build ./...
go test ./internal/api/... ./internal/connector/... -count=1
```

预期：`gofmt -l` 无输出；build 通过；测试全绿（含任务 1 新增用例与既有 capture 用例）。

- [ ] **步骤 5：Commit**

```powershell
git add web/chat/src/pages/captureForm.ts web/chat/src/pages/CaptureSettings.test.ts web/chat/dist
git commit --trailer "Co-authored-by: Cursor <cursoragent@cursor.com>" -m "refactor(web): 移除连接器页废弃的凭证辅助代码并重建 dist"
```

---

## 人工走查（实现完成后）

启动一个隔离实例（独立端口 / 数据目录），用管理员身份在浏览器与移动视口走查：

1. 业务系统：空态文案 →「接入业务系统」→ 仅连接编号 / 服务地址 / 接口文档（无任何鉴权字段）→ 不填文档被行内拦截 → 填一个公开示例文档保存 → 进入第二步工具勾选 → 勾选「需本人登录 / 需审批」→ 完成；重开该连接，勾选回显正确。
2. 插件：只需编号 + 服务地址；地址不通时出现中文 Toast（非 `invalid_plugin` 原文）。
3. 错误文档、文档链接不可达、工具重名，分别出现第 5 节定义的中文提示。
4. 行「⋯」菜单：编辑、删除（删除有 ConfirmDialog，确认后工具移除）。
5. 回归：在「助手功能 / Tools」给某连接配置自定义 capture 后，回到连接器页只改服务地址保存，capture 不被清空（任务 1 的后端保证）。
6. 暗色模式与 390px 移动视口无横向溢出、触摸目标 ≥40px。

---

## 自检对照（规格 → 任务）

- §1.2 移除默认凭证编辑、不展示不回传：任务 5（表单无鉴权字段）、6/7（提交体不含 auth）、8（删死代码）。
- §2 capture 省略保留后端：任务 1。
- §3.1 外壳 / §3.2 列表与空态 / §3.4 删除确认：任务 4。
- §3.3 两步 Modal 与权限勾选 / 回显 / 二次 PUT：任务 3（纯逻辑）、5（组件）、6/7（接线）。
- §3.5 错误 Toast：任务 2（文案映射）、6/7（`connectorErrorText`）。
- §4 术语人话化：任务 2 文案 + 任务 4/5 组件消费。
- §5 错误码表：任务 2。
- §7 文件结构 / 删除旧导出：任务 3-8。
- §8 测试与验收：各任务单测 + 任务 8 全量回归 + 上节人工走查。