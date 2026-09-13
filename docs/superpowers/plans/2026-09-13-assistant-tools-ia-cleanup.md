# 助手功能 IA 收口实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 收缩「助手功能」为能力总表；把执行回调与登录捕获迁回连接编辑弹窗第一步「高级」；把按工具 MCP 导出迁到「对外提供能力」；选文件按钮化；技能文案与运营只读展示对齐规格。

**架构：** 以前端为主。连接 PUT 对 `execution_callback_url` 做与 capture 同策略的「省略保留」（小改 API 解包层）。`ConnectorEditorModal` 在 openapi/plugin 第一步挂高级区并经 `SavedConnection` / `onSavePermissions` 整表回传。`ToolsSettings` 删除组头连接设置与行上导出。`McpExportSettings` 新增按工具 export 段。共享 `FilePickerButton`。不改消息回调页。

**技术栈：** React、TypeScript、Vite、Vitest + jsdom、Go（仅 PUT 省略保留）、`web/chat`。命令：`cd web/chat && npm test`、`npx tsc --noEmit`、`npm run build`（`outDir` → `internal/ui/dist`）。Go：`go test ./internal/api/ -run TestPutConnector -count=1`。提交进 `real/main`，中文 Conventional Commits。

规格：`docs/superpowers/specs/2026-09-13-assistant-tools-ia-cleanup-design.md`

---

## 文件结构

新增：

- `web/chat/src/components/settings/FilePickerButton.tsx` — 按钮 + 隐藏 `input[type=file]` + 可选已选文件名/清除
- `web/chat/src/components/settings/FilePickerButton.test.tsx` — 点击触发、清除、disabled
- （可选）`web/chat/src/pages/McpExportToolsSection.tsx` — 若 `McpExportSettings.tsx` 过大再拆；默认允许先写在同文件

修改：

- `internal/api/server.go` — `execution_callback_url` 改为 `*string`，省略则读已有连接保留
- `internal/api/server_connector_capture_test.go`（或新建 `server_connector_callback_test.go`）— 省略保留 / 显式清空回归
- `web/chat/src/pages/connectorForms/types.ts` — `SavedConnection` openapi/plugin 增加 `executionCallbackUrl`、`auth?`
- `web/chat/src/components/settings/ConnectorEditorModal.tsx` + `.test.tsx` — 选文件原语、高级区、保存载荷
- `web/chat/src/pages/OpenApiSettings.tsx`、`PluginSettings.tsx` — `openEdit`/`putConnector` 回传 callback + auth
- `web/chat/src/pages/ToolsSettings.tsx` + `.test.tsx` — 删组头/导出；精简行；运营徽章；去 `getConnector` 拉详情（若不再需要）
- `web/chat/src/pages/CaptureSettingsFields.tsx` — 可继续用 `TOOLS.capture*` 键（不强制搬迁）
- `web/chat/src/pages/McpExportSettings.tsx` + `.test.tsx` — 按工具 export UI；改 intro
- `web/chat/src/pages/SkillsSettings.tsx` + `.test.tsx` / `strings.skills.test.ts`
- `web/chat/src/pages/ReadOnlyGate.tools-skills.test.tsx` — 运营零 checkbox、无人话写入口
- `web/chat/src/strings.ts` — `TOOLS` / `CONNECTORS` / `MCP_EXPORTS` / `SKILLS` / 共享选文件文案
- `web/chat/src/style.css` — 去框套框（`.settings-group` 等）
- `internal/ui/dist/**` — 末任务重建

不改：`WebhookSettings`（消息回调）、MCP OAuth、后端字段改名/删列、抽 `AssistantListShell`。

**ACL 澄清（写入测试）：** 运营仍不得请求 `GET /v0/agents/*`。技能「是否默认」徽章仅管理员在已知 `selected` 时展示；运营只展示名称/来源等，**不**用 disabled checkbox 冒充只读。

---

## 任务 1：选文件原语（连接弹窗 + 技能上传）

**文件：**
- 创建：`web/chat/src/components/settings/FilePickerButton.tsx`
- 创建：`web/chat/src/components/settings/FilePickerButton.test.tsx`
- 修改：`web/chat/src/strings.ts`（`CONNECTORS.chooseSpec` / `COMMON.chooseFile` / `COMMON.clearFile` 等，键名以本任务测试为准）
- 修改：`web/chat/src/components/settings/ConnectorEditorModal.tsx`
- 修改：`web/chat/src/components/settings/ConnectorEditorModal.test.tsx`
- 修改：`web/chat/src/pages/SkillsSettings.tsx`（页头与空态统一走按钮触发；隐藏 file input）

- [ ] **步骤 1：编写 FilePickerButton 失败测试**

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import { FilePickerButton } from './FilePickerButton'

describe('FilePickerButton', () => {
  it('clicking choose button activates hidden file input', async () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const onChange = vi.fn()
    const clickSpy = vi.fn()
    await act(async () => {
      createRoot(host).render(
        <FilePickerButton accept=".md,.zip" chooseLabel="选择文件" onFile={onChange} />,
      )
    })
    const input = host.querySelector('input[type="file"]') as HTMLInputElement
    expect(input).toBeTruthy()
    expect(input.hidden || input.getAttribute('hidden') !== null || input.classList.contains('sr-only') || input.style.display === 'none' || !input.checkVisibility?.()).toBeTruthy()
    // 简化：断言 input 存在且 tabIndex/aria 不抢焦点；按钮文案正确
    expect(host.textContent).toContain('选择文件')
    input.click = clickSpy
    const btn = [...host.querySelectorAll('button')].find((b) => b.textContent?.includes('选择文件'))
    await act(async () => { btn!.click() })
    expect(clickSpy).toHaveBeenCalled()
    host.remove()
  })

  it('shows fileName and clear calls onClear', async () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const onClear = vi.fn()
    await act(async () => {
      createRoot(host).render(
        <FilePickerButton
          accept=".json"
          chooseLabel="选择接口文档"
          clearLabel="清除已选"
          fileName="a.json"
          onFile={() => {}}
          onClear={onClear}
        />,
      )
    })
    expect(host.textContent).toContain('a.json')
    const clear = [...host.querySelectorAll('button')].find((b) => b.textContent?.includes('清除已选'))
    await act(async () => { clear!.click() })
    expect(onClear).toHaveBeenCalled()
    host.remove()
  })
})
```

（若 `checkVisibility` 在 jsdom 不可用：只断言 `input` 有 `hidden` 属性或 `className` 含视觉隐藏，且不作为主可见控件替代按钮。）

- [ ] **步骤 2：运行确认失败**

```bash
cd web/chat && npx vitest run src/components/settings/FilePickerButton.test.tsx
```

预期：FAIL（模块不存在）

- [ ] **步骤 3：实现 FilePickerButton**

```tsx
import { useRef, type ChangeEvent } from 'react'
import { Button } from '../ui'

export interface FilePickerButtonProps {
  accept: string
  chooseLabel: string
  clearLabel?: string
  fileName?: string | null
  disabled?: boolean
  onFile: (file: File | undefined) => void
  onClear?: () => void
  'aria-label'?: string
}

export function FilePickerButton(props: FilePickerButtonProps) {
  const ref = useRef<HTMLInputElement>(null)
  const onChange = (e: ChangeEvent<HTMLInputElement>) => {
    props.onFile(e.target.files?.[0])
    e.target.value = ''
  }
  return (
    <div className="file-picker">
      <input
        ref={ref}
        type="file"
        accept={props.accept}
        hidden
        disabled={props.disabled}
        aria-label={props['aria-label'] ?? props.chooseLabel}
        onChange={onChange}
      />
      <Button
        type="button"
        variant="secondary"
        size="sm"
        disabled={props.disabled}
        onClick={() => ref.current?.click()}
      >
        {props.chooseLabel}
      </Button>
      {props.fileName ? (
        <span className="file-picker-name">
          {props.fileName}
          {props.onClear && props.clearLabel ? (
            <Button type="button" variant="ghost" size="sm" disabled={props.disabled} onClick={props.onClear}>
              {props.clearLabel}
            </Button>
          ) : null}
        </span>
      ) : null}
    </div>
  )
}
```

在 `strings.ts` 增加（示例）：

```ts
// CONNECTORS
chooseSpec: '选择接口文档',
clearSpec: '清除已选', // 若已有 specRemoveFile 则复用 CONNECTORS.specRemoveFile，不要重复键
```

技能页：`SKILLS.upload` 已存在；页头与空态都用 `FilePickerButton` 或同等「按钮 → `fileRef.click()`」；页头**不得**再渲染可见原生 file 控件。

- [ ] **步骤 4：改 ConnectorEditorModal 的 OpenAPI 文档选择**

把裸 `<Input type="file" …>` 换成 `FilePickerButton`（`accept=".json,.yaml,.yml"`，`chooseLabel={CONNECTORS.chooseSpec}`，`fileName={specFileName}`，`onClear={removeSpecFile}`，`onFile` 接到现有 `onSpecFile` 逻辑）。保留文件/URL 互斥。

更新 `ConnectorEditorModal.test.tsx`：用按钮文案找控件；`I-2 spec file removal` 仍通过清除按钮。

- [ ] **步骤 5：改 SkillsSettings 页头上传**

页头：隐藏 `<input type="file" hidden ref={fileRef} />` + `Button`「上传技能」触发；去掉裸 file 作为主控件。空态已按钮化则对齐同一 `fileRef`。

- [ ] **步骤 6：跑测试并 commit**

```bash
cd web/chat && npx vitest run src/components/settings/FilePickerButton.test.tsx src/components/settings/ConnectorEditorModal.test.tsx src/pages/SkillsSettings.test.tsx
```

```bash
git add web/chat/src/components/settings/FilePickerButton.tsx web/chat/src/components/settings/FilePickerButton.test.tsx web/chat/src/components/settings/ConnectorEditorModal.tsx web/chat/src/components/settings/ConnectorEditorModal.test.tsx web/chat/src/pages/SkillsSettings.tsx web/chat/src/strings.ts
git commit -m "$(cat <<'EOF'
feat(web): 选文件改为按钮触发隐藏 input

EOF
)"
```

（Windows PowerShell 可用 UTF-8 无 BOM 的 `-F m.txt` 方式，与仓库既有习惯一致。）

---

## 任务 2：PUT `execution_callback_url` 省略保留（后端）

**文件：**
- 修改：`internal/api/server.go`（`handlePutConnector` 请求体）
- 测试：`internal/api/server_connector_callback_test.go`（新建）或并入既有 capture 测试文件

- [ ] **步骤 1：编写失败测试**

```go
func TestPutConnectorOmitsExecutionCallbackPreservesExisting(t *testing.T) {
	st := newTestStore(t) // 与 capture 测试相同的 store/server 脚手架；复用 putConnectorJSON / apiNewServer
	// 1) 创建 openapi 连接并带 execution_callback_url
	putConnectorJSON(t, h, "cb1", map[string]any{
		"type": "openapi",
		"base_url": "https://example.com",
		"spec_content": /* 最小合法 spec，同其它测试 */,
		"execution_callback_url": "https://gw.example/execute",
	})
	// 2) 省略该字段再 PUT（只改 base_url）
	putConnectorJSON(t, h, "cb1", map[string]any{
		"type": "openapi",
		"base_url": "https://example.com/v2",
		// 无 execution_callback_url
	})
	got, err := st.GetConnector("cb1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ExecutionCallbackURL != "https://gw.example/execute" {
		t.Fatalf("callback not preserved: %q", got.ExecutionCallbackURL)
	}
}

func TestPutConnectorEmptyExecutionCallbackClears(t *testing.T) {
	// 同上先写入非空，再显式 ""，断言清空
}
```

脚手架函数名以 `server_connector_capture_test.go` 已有 helper 为准，不要重复定义冲突符号。

- [ ] **步骤 2：运行确认失败**

```powershell
$env:PATH = "C:\Users\Administrator\.local\go1.25.0\bin;" + $env:PATH
go test ./internal/api/ -run 'TestPutConnectorOmitsExecutionCallback|TestPutConnectorEmptyExecutionCallback' -count=1
```

预期：省略用例 FAIL（第二次 PUT 后 URL 被清空）。

- [ ] **步骤 3：实现省略保留**

在 `handlePutConnector` 将：

```go
ExecutionCallbackURL string `json:"execution_callback_url"`
```

改为：

```go
ExecutionCallbackURL *string `json:"execution_callback_url"`
```

构造 `ApplyInput` 前：

```go
callbackURL := ""
if body.ExecutionCallbackURL != nil {
	callbackURL = strings.TrimSpace(*body.ExecutionCallbackURL)
} else if existing, err := s.Store.GetConnector(id); err == nil {
	callbackURL = existing.ExecutionCallbackURL
}
// ApplyInput.ExecutionCallbackURL: callbackURL
```

注意：同一 handler 里若已为 capture 取过 `existing`，可合并一次 `GetConnector`，避免双读；不要改变 capture 逻辑。

- [ ] **步骤 4：测试通过并 commit**

```powershell
go test ./internal/api/ -run 'TestPutConnectorOmitsExecutionCallback|TestPutConnectorEmptyExecutionCallback|TestPutConnectorOmitsCapture' -count=1
```

```bash
git add internal/api/server.go internal/api/server_connector_callback_test.go
git commit -m "$(cat <<'EOF'
fix(api): PUT 省略 execution_callback_url 时保留原值

EOF
)"
```

---

## 任务 3：连接弹窗第一步「高级」+ 助手功能去掉组头块

**文件：**
- 修改：`web/chat/src/pages/connectorForms/types.ts`
- 修改：`web/chat/src/components/settings/ConnectorEditorModal.tsx` + `.test.tsx`
- 修改：`web/chat/src/pages/OpenApiSettings.tsx`、`PluginSettings.tsx`
- 修改：`web/chat/src/strings.ts`（`CONNECTORS.advanced`、`executionCallback`、`executionCallbackHint`）
- 修改：`web/chat/src/pages/ToolsSettings.tsx` + `.test.tsx`（删除 callback/capture 组头与相关 state）
- 修改：`web/chat/src/pages/ReadOnlyGate.tools-skills.test.tsx`（若文案依赖）

### 3.A 类型与 Modal

- [ ] **步骤 1：扩展类型**

```ts
// types.ts — SavedConnection
| {
    kind: 'openapi'
    id: string
    baseUrl: string
    spec?: { content?: string; url?: string }
    importFormat: ImportFormat
    executionCallbackUrl: string
    auth?: ConnectorAuth
  }
| {
    kind: 'plugin'
    id: string
    baseUrl: string
    executionCallbackUrl: string
    auth?: ConnectorAuth
  }
| { kind: 'mcp'; id: string; mcp: MCPConfig }
```

`ConnectorEditorInitial` 增加：

```ts
executionCallbackUrl?: string
auth?: ConnectorAuth // 用于回显 capture
```

`onSavePermissions` 签名扩展为（第四参，非 mcp 必传当前高级快照）：

```ts
onSavePermissions: (
  id: string,
  loginNames: string[],
  approvalNames: string[],
  advanced?: { executionCallbackUrl: string; auth?: ConnectorAuth },
) => Promise<void>
```

- [ ] **步骤 2：Modal 状态与 UI**

仅 `!isMcp`：在第一步表单底部加

```tsx
<details className="settings-advanced">
  <summary>{CONNECTORS.advanced}</summary>
  <Field label={CONNECTORS.executionCallback} hint={CONNECTORS.executionCallbackHint}>
    <Input value={executionCallbackUrl} disabled={saving}
      onChange={(e) => setExecutionCallbackUrl(e.target.value)}
      placeholder="https://enterprise.example/baize/execute" />
  </Field>
  <CaptureSettingsFields
    connectorId={id || 'new'}
    connectorType={kind === 'plugin' ? 'http' : 'openapi'}
    draft={captureDraft}
    onDraftChange={(patch) => setCaptureDraft((d) => ({ ...d, ...patch }))}
  />
</details>
```

定稿文案：

```ts
advanced: '高级',
executionCallback: '企业统一执行地址',
executionCallbackHint:
  '选填。不是业务 API 前缀，也不是「消息回调」。有值时工具调用改 POST 到此地址，由企业网关执行。多数场景留空。',
```

`open` 重置时：`setExecutionCallbackUrl(init.executionCallbackUrl ?? '')`，`setCaptureDraft(captureToDraft(init.auth?.capture))`。

`saveInfo` 构造 `conn` 时带上：

```ts
executionCallbackUrl: executionCallbackUrl.trim(),
auth: mergeAuthWithCapture(initial.auth /* 或当前 base */, captureDraft),
```

`finish()` 调用：

```ts
await props.onSavePermissions(
  id.trim(),
  isMcp ? [] : loginNames,
  approvalNames,
  isMcp ? undefined : {
    executionCallbackUrl: executionCallbackUrl.trim(),
    auth: mergeAuthWithCapture(initialRef.current.auth, captureDraft),
  },
)
```

**重要：** 用户点「下一步到权限」不保存时，`finish` 仍必须带上 Modal 内当前高级草稿（含未点保存的编辑）。

- [ ] **步骤 3：OpenApiSettings / PluginSettings 接线**

`openEdit`：从 `ConnectorInfo` 填 `executionCallbackUrl`、`auth`；`setSavedConn` 同步带上。

`handleSaveInfo`：

```ts
await putConnector(conn.id, {
  type: 'openapi', // 或 http
  base_url: conn.baseUrl,
  // …既有 spec 字段
  execution_callback_url: conn.executionCallbackUrl,
  auth: conn.auth,
})
```

`handleSavePermissions`：

```ts
await putConnector(id, {
  type: 'openapi',
  base_url: savedConn.baseUrl,
  require_login: loginNames,
  require_approval: approvalNames,
  execution_callback_url: advanced?.executionCallbackUrl ?? savedConn.executionCallbackUrl,
  auth: advanced?.auth ?? savedConn.auth,
})
```

Plugin 同理（`type: 'http'`）。MCP 页不传 advanced。

- [ ] **步骤 4：Modal 测试**

在 `ConnectorEditorModal.test.tsx` 增加：

1. 高级 `<details>` 默认 `open === false`；可见「企业统一执行地址」与 capture 人话标签；MCP 无此块。
2. `onSaveInfo` 收到的对象含 `executionCallbackUrl` 与 `auth.capture`。
3. 仅走权限保存时 `onSavePermissions` 第四参带当前 URL。

- [ ] **步骤 5：从 ToolsSettings 删除组头块**

删除：`callbackDrafts` / `captureDrafts` / `callbackSaving` / `saveConnectorSettings` / `updateCaptureDraft` / `supportsExecutionCallback` 组头 UI；删除为组头服务的 `getConnector` 批量加载（若删除后无其它用途）；删除 `mergeAuthWithCapture` / `CaptureSettingsFields` 在本页的 import。

更新 `ToolsSettings.test.tsx`：

- 删除「CaptureSettingsFields 在 Tools 页高级折叠」「saveConnectorSettings PUT merge」用例，或改写为断言**页面上不再出现**「保存 Connector 设置」「执行回调 URL」「企业统一执行地址」。
- 保留添加工具 Modal、schema 折叠等无关用例。

- [ ] **步骤 6：跑测 commit**

```bash
cd web/chat && npx vitest run src/components/settings/ConnectorEditorModal.test.tsx src/pages/ToolsSettings.test.tsx
```

```bash
git commit -m "$(cat <<'EOF'
feat(web): 执行回调与登录捕获迁入连接高级区

EOF
)"
```

---

## 任务 4：助手功能收缩（导出下拉、边框、文案、运营只读）

**文件：**
- 修改：`web/chat/src/pages/ToolsSettings.tsx` + `.test.tsx`
- 修改：`web/chat/src/strings.ts`（`TOOLS.description`、行标签）
- 修改：`web/chat/src/style.css`（树去多层边框）
- 修改：`web/chat/src/pages/ReadOnlyGate.tools-skills.test.tsx`

- [ ] **步骤 1：文案**

```ts
description: '查看并管理助手可调用的功能：启停、用前是否需登录，以及显示名与说明。',
enable: '启用',
requireLogin: '用前需登录',
requireApprovalBadge: '用前需确认',
editCopy: '编辑文案', // 若硬编码则改用键
techDetails: '技术详情',
```

行上：`需要登录` → `TOOLS.requireLogin`；徽章「需审批」→ `TOOLS.requireApprovalBadge`。方法/路径默认不常显：移入编辑区或 `<details><summary>{TOOLS.techDetails}</summary>`（与规格一致）。

- [ ] **步骤 2：移除 MCP 导出下拉与 MCP 写类提示行**

删除 `EXPORT_OPTIONS`、`onExportChange`、`toolExportMode`、行内 `<select>`、`MCP 写类工具…` 提示。

测试断言 admin 视图：

```ts
expect(host.textContent).not.toContain('MCP 导出')
expect(host.querySelectorAll('select').length).toBe(0) // 添加工具 Modal 未打开时
```

- [ ] **步骤 3：运营只读加固**

运营：无启用/登录 checkbox；用 Badge/文案展示「已启用 / 已停用」「用前需登录」「用前需确认」。无添加、无组批量、无删除、无编辑。

更新 `ReadOnlyGate.tools-skills.test.tsx`：

```ts
expect(host.querySelectorAll('input[type="checkbox"]').length).toBe(0)
expect(host.textContent).not.toContain('添加')
expect(host.textContent).not.toContain(TOOLS.requireLogin) // 若开关文案仅 admin；或断言无「启用」开关——按实现选稳定断言
```

（若只读徽章也含「用前需登录」四字，则改为断言无 `input[type=checkbox]` + 无「全部启用」。）

- [ ] **步骤 4：样式去框套框**

在 `style.css`：

- `.settings-group`：去掉实线边框或改为仅底部分隔；nested 组避免再套圆角框。
- `.settings-tool-row` / list：用间距与弱分隔，避免框套框。

目视：树仍可读，不再「盒子套盒子」。

- [ ] **步骤 5：「添加」降为次要**

页头添加按钮改为 `variant="secondary"`（或移到高级/次要区）；主视觉不强调手工 extra。

- [ ] **步骤 6：测试 + commit**

```bash
cd web/chat && npx vitest run src/pages/ToolsSettings.test.tsx src/pages/ReadOnlyGate.tools-skills.test.tsx
```

```bash
git commit -m "$(cat <<'EOF'
refactor(web): 收缩助手功能页并加固运营只读

EOF
)"
```

---

## 任务 5：对外提供能力 — 按工具 export

**文件：**
- 修改：`web/chat/src/strings.ts`（`MCP_EXPORTS`）
- 修改：`web/chat/src/pages/McpExportSettings.tsx` + `.test.tsx`

- [ ] **步骤 1：文案测试（可写在 McpExport 测试或 strings 测）**

```ts
expect(MCP_EXPORTS.toolsExportTitle).toBe('按功能是否对外导出')
expect(MCP_EXPORTS.exportDefault).toBe('跟随默认规则')
expect(MCP_EXPORTS.exportForceAllow).toBe('必须导出')
expect(MCP_EXPORTS.exportForceDeny).toBe('禁止导出')
expect(MCP_EXPORTS.introToolsLink).not.toMatch(/助手功能/)
```

更新：

```ts
intro: '…', // 可微调
introToolsLink: '下方可按功能覆盖默认导出规则；未覆盖的功能跟随系统默认。',
toolsExportTitle: '按功能是否对外导出',
toolsExportIntro: '选择每个功能在对外 MCP 中的导出策略。',
toolsExportEmpty: '还没有可配置的功能。',
toolsExportSearch: '搜索功能',
exportDefault: '跟随默认规则',
exportForceAllow: '必须导出',
exportForceDeny: '禁止导出',
toastExportSaved: '已更新导出策略',
```

- [ ] **步骤 2：UI 段**

在身份/密钥区块之前或之后增加一节（admin 可写；若本页整体已 gate 则跟随页）：

- `useEffect`：`listTools()` 载入列表。
- 搜索过滤 `title/name/description`。
- 每行：显示名 + `<Select>` 三态；`value` 为 `t.export ?? 'default'`（空当 default）。
- `onChange` → `patchTool(name, { export })` → toast 成功/错误（`mcpExportErrorText` 或 `toolErrorText`）。

MCP 源工具可保留一行弱提示：「MCP 写类工具即使设为必须导出也不会对外提供」（人话，勿堆在助手功能页）。

- [ ] **步骤 3：组件测试**

Mock `listTools` / `patchTool`：渲染后可见「按功能是否对外导出」；改 select 调用 `patchTool` 且参数为 `force_allow`。

- [ ] **步骤 4：commit**

```bash
cd web/chat && npx vitest run src/pages/McpExportSettings.test.tsx
git commit -m "$(cat <<'EOF'
feat(web): 对外提供能力页支持按工具配置导出

EOF
)"
```

---

## 任务 6：技能文案 + 运营只读展示

**文件：**
- 修改：`web/chat/src/strings.ts`、`strings.skills.test.ts`
- 修改：`web/chat/src/pages/SkillsSettings.tsx` + `.test.tsx`
- 修改：`web/chat/src/pages/ReadOnlyGate.tools-skills.test.tsx`

- [ ] **步骤 1：文案**

```ts
saveDefaults: '保存为默认技能',
saveDefaultsHint: '对新开的对话生效；对话里用 @ / / 仍可临时选用。',
toastSaved: '默认技能已保存',
defaultBadge: '默认',
notDefaultBadge: '非默认', // 若运营不展示可仅 admin 用 defaultBadge
```

`strings.skills.test.ts`：`expect(SKILLS.saveDefaults).toBe('保存为默认技能')`；断言 `saveDefaultsHint` 存在。

- [ ] **步骤 2：SkillsSettings UI**

- 保存按钮旁（或下方）渲染 `SKILLS.saveDefaultsHint`。
- **管理员**：保留 checkbox 勾选默认。
- **运营（`readOnly`）**：**不渲染** `input[type=checkbox]`；列表项用标题 + 来源 Badge；不展示默认勾选 UI（因无 getAgent 数据，不要用 disabled checkbox）。
- 工具名摘要：移入 `settings-muted` 次要行或缩短。

- [ ] **步骤 3：更新 ReadOnlyGate**

```ts
expect(host.textContent).not.toContain('保存为默认技能')
expect(host.textContent).not.toContain('保存默认勾选')
expect(host.querySelectorAll('input[type="checkbox"]').length).toBe(0)
expect(host.querySelector('input[type="file"]')).toBeNull()
expect(urls.some((u) => u.startsWith('/v0/agents/'))).toBe(false)
```

删除「checkboxes present but disabled」断言。

- [ ] **步骤 4：commit**

```bash
cd web/chat && npx vitest run src/strings.skills.test.ts src/pages/SkillsSettings.test.tsx src/pages/ReadOnlyGate.tools-skills.test.tsx
git commit -m "$(cat <<'EOF'
fix(web): 技能默认保存文案与运营只读展示

EOF
)"
```

---

## 任务 7：全量门禁 + 嵌入 dist

**文件：** `internal/ui/dist/**`（及本计划未提交的残余）

- [ ] **步骤 1：前端全量**

```bash
cd web/chat && npm test && npx tsc --noEmit && npm run build
```

预期：全绿；`internal/ui/dist` 更新。

- [ ] **步骤 2：Go 相关回归**

```powershell
$env:PATH = "C:\Users\Administrator\.local\go1.25.0\bin;" + $env:PATH
go test ./internal/api/ -count=1
```

- [ ] **步骤 3：手动验收清单（执行者自检，不必自动化）**

1. Admin：连接编辑 → 高级填写企业统一执行地址与捕获 → 保存 → 助手功能页无该块 → 再打开连接编辑值仍在。
2. Admin：对外提供能力可改三态导出；助手功能无导出下拉。
3. 运营：助手功能/技能零写控件、技能无 checkbox。
4. 消息回调页未改动。

- [ ] **步骤 4：commit + push real**

```bash
git add internal/ui/dist web/chat
git commit -m "$(cat <<'EOF'
chore(ui): 重建嵌入前端产物（助手功能 IA 收口）

EOF
)"
git push real main
```

---

## 自检（对照规格）

| 规格条款 | 任务 |
|----------|------|
| §4 助手功能收缩 / 去组头 / 去导出 / 去边框 | 3、4 |
| §5 连接高级 + 选文件 + 废止清空 callback | 1、2、3 |
| §6 导出页按工具配置 | 5 |
| §7 技能上传/文案/运营 | 1、6 |
| §8 运营硬约束测试 | 4、6 |
| §9 文案键 | 遍布 1–6 |
| §10 测试 + dist | 7 |
| 不做消息回调合并 | 无任务改 Webhook |

占位符扫描：无 TODO/待定步骤。类型名统一：`executionCallbackUrl`（TS）、`execution_callback_url`（JSON）、`CONNECTORS.executionCallback`（文案）。
