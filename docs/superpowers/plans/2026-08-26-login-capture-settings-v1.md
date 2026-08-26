# 登录捕获设置 v1 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** Tools 页为 OpenAPI / HTTP Connector 配置 `auth.capture`；插件 / OpenAPI 设置页保存时不重置 capture；列表可显示 capture 状态。

**架构：** 在 `captureForm.ts` 集中纯函数（类型判断、摘要文案、preserve merge）；新建 `CaptureSettingsFields` 供 Tools 页复用；Plugin / OpenAPI 页在 `PUT` 前 `mergeAuthPreserveCapture(built, editingConnector.auth)`；修正过时 Go API 测试以匹配 HTTP capture 已落地行为。

**技术栈：** React 19 + TypeScript + Vitest（`web/chat`）；Go 测试仅改 `internal/api/server_test.go` 断言。

**规格：** `docs/superpowers/specs/2026-08-26-login-capture-settings-v1-design.md`

**Git：** 建议分支 `feat/login-capture-settings-v1`；过程提交进 `baize_real`。

**环境（Windows，Go 测试任务）：**
```powershell
$env:PATH = "$env:USERPROFILE\sdk\go\bin;" + $env:PATH
$env:GOTOOLCHAIN = "go1.25.0"
$env:GOPROXY = "https://goproxy.cn,direct"
```

**前端测试 / 构建：**
```powershell
cd web/chat
npm run test
npm run build
```

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `web/chat/src/pages/captureForm.ts` | 扩展：`connectorSupportsLoginCapture`、`hasStoredCapture`、`captureSummaryLabel`、`mergeAuthPreserveCapture` |
| `web/chat/src/pages/CaptureSettings.test.ts` | 上述纯函数单测 |
| `web/chat/src/pages/CaptureSettingsFields.tsx`（新建） | Tools 页 capture 五字段 + 分类型 hint |
| `web/chat/src/pages/ToolsSettings.tsx` | 用共享函数/组件；HTTP 展示 capture |
| `web/chat/src/pages/PluginSettings.tsx` | preserve capture；列表摘要；页顶说明 |
| `web/chat/src/pages/PluginSettings.test.ts` | preserve merge 集成形状测试 |
| `web/chat/src/pages/OpenApiSettings.tsx` | preserve capture；列表摘要 |
| `internal/api/server_test.go` | 重命名并修正 HTTP PUT capture 断言 |
| `docs/superpowers/specs/2026-08-26-login-capture-settings-v1-design.md` | 状态改为已批准 |

---

### 任务 1：`captureForm` 纯函数扩展

**文件：**
- 修改：`web/chat/src/pages/captureForm.ts`
- 修改：`web/chat/src/pages/CaptureSettings.test.ts`

- [ ] **步骤 1：编写失败测试**

在 `CaptureSettings.test.ts` 追加：

```ts
import {
  buildCaptureFromDraft,
  captureSummaryLabel,
  captureToDraft,
  connectorSupportsLoginCapture,
  mergeAuthPreserveCapture,
  mergeAuthWithCapture,
  parsePathLines,
} from './captureForm'

describe('connectorSupportsLoginCapture', () => {
  it('allows openapi and http only', () => {
    expect(connectorSupportsLoginCapture('openapi')).toBe(true)
    expect(connectorSupportsLoginCapture('http')).toBe(true)
    expect(connectorSupportsLoginCapture('mcp')).toBe(false)
    expect(connectorSupportsLoginCapture(undefined)).toBe(false)
  })
})

describe('captureSummaryLabel', () => {
  it('returns null when capture empty', () => {
    expect(captureSummaryLabel(undefined)).toBeNull()
    expect(captureSummaryLabel({})).toBeNull()
  })

  it('labels disabled capture', () => {
    expect(captureSummaryLabel({ tool_name_glob: '__none__' })).toBe('捕获已关闭')
  })

  it('labels custom glob', () => {
    expect(captureSummaryLabel({ tool_name_glob: 'custom_*' })).toBe('捕获 custom_*')
  })

  it('labels default when only paths set', () => {
    expect(captureSummaryLabel({ token_json_paths: ['accessToken'] })).toBe('捕获（默认 *login*）')
  })
})

describe('mergeAuthPreserveCapture', () => {
  it('copies capture from existing auth onto built auth', () => {
    const built = { mode: 'static' as const, static: { headers: { Authorization: 'Bearer x' } } }
    const existing = {
      mode: 'static' as const,
      capture: { tool_name_glob: 'custom_*', token_json_paths: ['accessToken'] },
    }
    const merged = mergeAuthPreserveCapture(built, existing)
    expect(merged.capture?.tool_name_glob).toBe('custom_*')
    expect(merged.static?.headers?.Authorization).toBe('Bearer x')
  })

  it('preserves __none__ disable flag', () => {
    const merged = mergeAuthPreserveCapture(
      { mode: 'static' },
      { mode: 'static', capture: { tool_name_glob: '__none__' } },
    )
    expect(merged.capture?.tool_name_glob).toBe('__none__')
  })

  it('does not attach capture when existing had none', () => {
    const merged = mergeAuthPreserveCapture({ mode: 'static' }, { mode: 'static' })
    expect(merged.capture).toBeUndefined()
  })
})
```

- [ ] **步骤 2：运行测试验证失败**

```powershell
cd web/chat
npm run test -- src/pages/CaptureSettings.test.ts
```

预期：FAIL（`connectorSupportsLoginCapture is not exported` 或类似）

- [ ] **步骤 3：实现纯函数**

在 `captureForm.ts` 追加（保留现有 export）：

```ts
import type { ConnectorAuth } from '../api'

export function connectorSupportsLoginCapture(type: string | undefined): boolean {
  return type === 'openapi' || type === 'http'
}

function hasStoredCapture(capture: ConnectorAuth['capture'] | undefined): boolean {
  if (!capture) return false
  if ((capture.tool_name_glob ?? '').trim() === '__none__') return true
  if ((capture.tool_name_glob ?? '').trim() !== '') return true
  if ((capture.token_json_paths?.length ?? 0) > 0) return true
  if ((capture.label_json_paths?.length ?? 0) > 0) return true
  if ((capture.header_template ?? '').trim() !== '') return true
  if ((capture.default_scheme ?? '').trim() !== '') return true
  return false
}

export function captureSummaryLabel(capture: ConnectorAuth['capture'] | undefined): string | null {
  if (!hasStoredCapture(capture)) return null
  const glob = (capture?.tool_name_glob ?? '').trim()
  if (glob === '__none__') return '捕获已关闭'
  if (glob !== '') {
    const shown = glob.length > 32 ? `${glob.slice(0, 29)}…` : glob
    return `捕获 ${shown}`
  }
  return '捕获（默认 *login*）'
}

export function mergeAuthPreserveCapture(
  built: ConnectorAuth,
  existing: ConnectorAuth | undefined,
): ConnectorAuth {
  if (!hasStoredCapture(existing?.capture)) return built
  return { ...built, capture: { ...existing!.capture! } }
}
```

- [ ] **步骤 4：运行测试验证通过**

```powershell
cd web/chat
npm run test -- src/pages/CaptureSettings.test.ts
```

预期：PASS

- [ ] **步骤 5：Commit**

```powershell
git add web/chat/src/pages/captureForm.ts web/chat/src/pages/CaptureSettings.test.ts
git commit -m "feat(web): captureForm 支持类型判断与 preserve merge"
```

---

### 任务 2：`CaptureSettingsFields` 组件 + Tools 页 HTTP 展示

**文件：**
- 创建：`web/chat/src/pages/CaptureSettingsFields.tsx`
- 修改：`web/chat/src/pages/ToolsSettings.tsx`

- [ ] **步骤 1：创建 `CaptureSettingsFields.tsx`**

```tsx
import type { CaptureDraft } from './captureForm'

export interface CaptureSettingsFieldsProps {
  connectorId: string
  connectorType: 'openapi' | 'http'
  draft: CaptureDraft
  onDraftChange: (patch: Partial<CaptureDraft>) => void
}

export function CaptureSettingsFields({
  connectorId,
  connectorType,
  draft,
  onDraftChange,
}: CaptureSettingsFieldsProps) {
  const captureHint =
    connectorType === 'http'
      ? '登录捕获：侧车中名称匹配 glob 的工具 invoke 成功后写入会话身份（如 login）。'
      : '登录捕获：匹配 glob 的 operation 响应 JSON 写入会话身份。'

  return (
    <>
      <label className="settings-field">
        <span className="settings-label">登录捕获 tool_name_glob</span>
        <input
          className="settings-input"
          value={draft.toolNameGlob}
          onChange={(e) => onDraftChange({ toolNameGlob: e.target.value })}
          placeholder="*login*（__none__ 关闭）"
        />
      </label>
      <label className="settings-field">
        <span className="settings-label">token_json_paths（每行一条）</span>
        <textarea
          className="settings-input"
          rows={3}
          value={draft.tokenPathsText}
          onChange={(e) => onDraftChange({ tokenPathsText: e.target.value })}
          placeholder="accessToken&#10;data.token"
        />
      </label>
      <label className="settings-field">
        <span className="settings-label">label_json_paths（每行一条）</span>
        <textarea
          className="settings-input"
          rows={2}
          value={draft.labelPathsText}
          onChange={(e) => onDraftChange({ labelPathsText: e.target.value })}
          placeholder="email"
        />
      </label>
      <label className="settings-field">
        <span className="settings-label">header_template</span>
        <input
          className="settings-input"
          value={draft.headerTemplate}
          onChange={(e) => onDraftChange({ headerTemplate: e.target.value })}
          placeholder="Bearer {{token}}"
        />
      </label>
      <label className="settings-field">
        <span className="settings-label">default_scheme（可选）</span>
        <input
          className="settings-input"
          value={draft.defaultScheme}
          onChange={(e) => onDraftChange({ defaultScheme: e.target.value })}
          placeholder="bearer"
        />
      </label>
      <p className="settings-hint">{captureHint}</p>
    </>
  )
}
```

- [ ] **步骤 2：修改 `ToolsSettings.tsx`**

1. 删除本地 `supportsLoginCapture` 函数。
2. 从 `captureForm` import `connectorSupportsLoginCapture`。
3. 从 `./CaptureSettingsFields` import `CaptureSettingsFields`。
4. 将所有 `supportsLoginCapture(connectorMeta[...])` 替换为 `connectorSupportsLoginCapture(connectorMeta[...]?.type)`。
5. 在 `saveConnectorSettings` 中：`connectorSupportsLoginCapture(meta.type)` 决定是否 `mergeAuthWithCapture`。
6. 用 `<CaptureSettingsFields>` 替换原 capture 五字段 JSX 块；`connectorType` 为 `meta.type === 'http' ? 'http' : 'openapi'`。
7. 组底 hint 保留执行回调一句：「执行回调：invoke 走企业统一 URL。」（capture 类型 hint 已在组件内）

- [ ] **步骤 3：运行前端测试与 build**

```powershell
cd web/chat
npm run test
npm run build
```

预期：PASS

- [ ] **步骤 4：Commit**

```powershell
git add web/chat/src/pages/CaptureSettingsFields.tsx web/chat/src/pages/ToolsSettings.tsx
git commit -m "feat(web): Tools 页 HTTP Connector 展示登录捕获表单"
```

---

### 任务 3：PluginSettings preserve capture + 列表摘要

**文件：**
- 修改：`web/chat/src/pages/PluginSettings.tsx`
- 修改：`web/chat/src/pages/PluginSettings.test.ts`

- [ ] **步骤 1：编写失败测试**

在 `PluginSettings.test.ts` 追加：

```ts
import { mergeAuthPreserveCapture } from './captureForm'

describe('plugin settings preserve capture', () => {
  it('mergeAuthPreserveCapture keeps custom glob when editing auth only', () => {
    const validatedAuth = {
      mode: 'static' as const,
      static: { headers: { Authorization: 'Bearer new' } },
    }
    const existingAuth = {
      mode: 'static' as const,
      capture: { tool_name_glob: 'custom_*' },
    }
    const auth = mergeAuthPreserveCapture(validatedAuth, existingAuth)
    expect(auth.capture?.tool_name_glob).toBe('custom_*')
    expect(auth.static?.headers?.Authorization).toBe('Bearer new')
  })
})
```

- [ ] **步骤 2：运行测试验证通过**（纯函数已在任务 1 实现，此测试应直接绿）

```powershell
cd web/chat
npm run test -- src/pages/PluginSettings.test.ts
```

- [ ] **步骤 3：修改 `PluginSettings.tsx`**

1. import `captureSummaryLabel`、`mergeAuthPreserveCapture` from `./captureForm`。
2. 新增 state：`const [editingConnector, setEditingConnector] = useState<ConnectorInfo | null>(null)`。
3. `openCreate`：`setEditingConnector(null)`。
4. `openEdit(c)`：`setEditingConnector(c)`。
5. `onSubmit` 中 `putConnector` 的 `auth` 改为：
   ```ts
   auth: mergeAuthPreserveCapture(validated.auth, editingConnector?.auth),
   ```
6. 列表行 `requireParts` 后：若 `captureSummaryLabel(info.auth?.capture)` 非 null，push 到 `requireParts` 或单独 `<span>`。
7. 页顶 `settings-meta` 增加：「登录捕获与执行回调 URL 在 Tools 页按 Connector 配置。」

- [ ] **步骤 4：运行测试与 build**

```powershell
cd web/chat
npm run test
npm run build
```

- [ ] **步骤 5：Commit**

```powershell
git add web/chat/src/pages/PluginSettings.tsx web/chat/src/pages/PluginSettings.test.ts
git commit -m "fix(web): 插件设置保存时保留 Tools 页配置的 capture"
```

---

### 任务 4：OpenApiSettings preserve capture + 列表摘要

**文件：**
- 修改：`web/chat/src/pages/OpenApiSettings.tsx`

- [ ] **步骤 1：修改 `OpenApiSettings.tsx`**

与任务 3 对称：

1. import `captureSummaryLabel`、`mergeAuthPreserveCapture` from `./captureForm`。
2. `editingConnector` state；`openCreate` / `openEdit` 维护。
3. `onSubmit`：`auth: mergeAuthPreserveCapture(validated.auth, editingConnector?.auth)`。
4. 列表行显示 `captureSummaryLabel(info.auth?.capture)`。

- [ ] **步骤 2：运行测试与 build**

```powershell
cd web/chat
npm run test
npm run build
```

- [ ] **步骤 3：Commit**

```powershell
git add web/chat/src/pages/OpenApiSettings.tsx
git commit -m "fix(web): OpenAPI 设置保存时保留 Tools 页配置的 capture"
```

---

### 任务 5：修正 Go API 测试

**文件：**
- 修改：`internal/api/server_test.go`

- [ ] **步骤 1：重命名并修正断言**

将 `TestPutHTTPPluginUsesIdentitiesNoCapture` 重命名为 `TestPutHTTPPluginPreservesCaptureAndUsesIdentity`。

替换 capture 断言块（约 L1861–L1863）：

```go
	if c.Auth.Capture.ToolNameGlob != "*login*" {
		t.Fatalf("HTTP PUT must persist Capture, got %+v", c.Auth.Capture)
	}
```

保留 invoke 使用 `Bearer FROM_ID` 的断言（身份消费路径不变）。

- [ ] **步骤 2：运行 Go 测试**

```powershell
go test ./internal/api/... -run TestPutHTTPPluginPreservesCaptureAndUsesIdentity -count=1
```

预期：PASS

- [ ] **步骤 3：Commit**

```powershell
git add internal/api/server_test.go
git commit -m "test(api): HTTP PUT 应持久化 capture 而非清空"
```

---

### 任务 6：规格状态与整体验收

**文件：**
- 修改：`docs/superpowers/specs/2026-08-26-login-capture-settings-v1-design.md`

- [ ] **步骤 1：规格状态改为已批准**

文首 `> 状态：待用户审查` → `> 状态：已批准（2026-08-26）`

- [ ] **步骤 2：全量前端 + Go 相关测试**

```powershell
cd web/chat; npm run test; npm run build
cd ../..
go test ./internal/api/... -count=1
```

- [ ] **步骤 3：手工冒烟（可选但推荐）**

1. 注册 HTTP 侧车 Connector。
2. Tools 页设 `tool_name_glob: custom_*` 并保存。
3. 插件页仅改 `base_url` 保存 → GET connector 仍含 `custom_*`。
4. 插件列表行显示「捕获 custom_*」。

- [ ] **步骤 4：Commit**

```powershell
git add docs/superpowers/specs/2026-08-26-login-capture-settings-v1-design.md
git commit -m "docs: 登录捕获设置 v1 规格标记已批准"
```

---

## 规格覆盖自检

| 规格需求 | 任务 |
|---------|------|
| Tools http + openapi capture UI | 任务 2 |
| Plugin/OpenAPI preserve capture | 任务 3、4 |
| 列表摘要 | 任务 3、4 |
| connectorSupportsLoginCapture / captureSummaryLabel / mergeAuthPreserveCapture | 任务 1 |
| CaptureSettingsFields | 任务 2 |
| HTTP/OpenAPI 分类型 hint | 任务 2（组件内） |
| 插件页 Tools 说明 | 任务 3 |
| 纯函数单测 | 任务 1、3 |
| API 测试修正 | 任务 5 |
| 规格状态 | 任务 6 |
| MCP 不展示 | 任务 2（connectorSupportsLoginCapture 排除 mcp） |
| 不做 MCP UI / 试跑 / duplicate 表单 | 未列入任务 ✓ |

---

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-26-login-capture-settings-v1.md`。两种执行方式：

**1. 子代理驱动（推荐）** — 每个任务一个新子代理，任务间审查，快速迭代。必需子技能：`subagent-driven-development`。

**2. 内联执行** — 当前会话用 `executing-plans` 批量执行并设检查点。必需子技能：`executing-plans`。

**选哪种方式？**
