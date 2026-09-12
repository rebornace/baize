# P3-D 助手组设置页人话化（模型 / 技能 / 助手功能）实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将「模型」「技能」「助手功能」三页迁到与账号/连接器一致的人话 + P3-A 原语体验（Modal / ConfirmDialog / Toast / 高级折叠），不改后端契约，保留 P2 运营只读 gate。

**架构：** 前端纯改造。三批独立可合并：D1 模型（Modal 编辑 + 高级折叠 + ConfirmDialog）→ D2 技能（标题/原语 + 删除确认）→ D3 助手功能（树保留、抽屉改 Modal、登录捕获人话化并收进高级、保存强制 mergeAuthWithCapture）。不抽 `AssistantListShell`。文案与错误映射进 `strings.ts` 的 `MODELS` / `SKILLS` / `TOOLS`。

**技术栈：** React、TypeScript、Vite、Vitest + jsdom、`react-dom/client` 渲染测试、lucide-react。目录 `web/chat`。命令：`npm test`（`vitest run`）、`npx tsc --noEmit`、`npm run build`。嵌入产物：`npm run build` 后复制/重建 `internal/ui/dist`（与既有流程一致）。

规格：`docs/superpowers/specs/2026-09-12-webui-refresh-p3d-assistant-pages-design.md`

---

## 文件结构

新增：

- `web/chat/src/strings.models.test.ts` — `modelErrorText` / `MODELS` 键存在性
- `web/chat/src/strings.skills.test.ts` — `skillErrorText`
- `web/chat/src/strings.tools.test.ts` — `toolErrorText`
- （可选拆分，若文件过大再拆）`web/chat/src/pages/ModelProfileEditorModal.tsx` — 模型新建/编辑 Modal；默认允许留在 `ModelSettings.tsx` 内只要不超过可读性

修改：

- `web/chat/src/strings.ts` — `MODELS` / `SKILLS` / `TOOLS` + 三个 `*ErrorText`
- `web/chat/src/pages/ModelSettings.tsx` + `ModelSettings.test.tsx`
- `web/chat/src/pages/ReadOnlyGate.model-runtime.test.tsx` — 断言人话标题、无写按钮
- `web/chat/src/pages/SkillsSettings.tsx` + 既有 `SkillsSettings.test.ts`（纯函数）+ 新增交互 `.test.tsx` 若需要
- `web/chat/src/pages/ToolsSettings.tsx` + `CaptureSettingsFields.tsx`
- `web/chat/src/pages/ReadOnlyGate.tools-skills.test.tsx`
- `web/chat/src/styles/components.css` — `.settings-advanced`（若尚无）用于 `<details>` 高级折叠
- `internal/ui/dist/**` — 每批末尾或总末重建

不改：`api.ts` 契约、聊天模型芯片、后端 Go。

---

## 批次 D1：模型

### 任务 1：`MODELS` 文案与 `modelErrorText`

**文件：**
- 修改：`web/chat/src/strings.ts`
- 测试：`web/chat/src/strings.models.test.ts`

- [ ] **步骤 1：编写失败测试**

```ts
import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import { MODELS, modelErrorText } from './strings'

describe('MODELS', () => {
  it('exposes humanized page copy', () => {
    expect(MODELS.title).toBe('模型')
    expect(MODELS.add).toBe('添加模型')
    expect(MODELS.advanced).toBe('高级')
    expect(MODELS.fieldBaseUrl).toBe('服务地址')
    expect(MODELS.fieldDisableThinking).toBe('禁用思考')
  })
})

describe('modelErrorText', () => {
  it('maps not_found', () => {
    expect(modelErrorText(new ApiError(404, 'not_found', 'x')).title).toBe(MODELS.errNotFound)
  })
  it('maps invalid_request', () => {
    expect(modelErrorText(new ApiError(400, 'invalid_request', 'name is required')).title).toBe(
      MODELS.errInvalidRequest,
    )
  })
  it('falls back via friendly shape for unknown', () => {
    const r = modelErrorText(new ApiError(500, 'internal_error', 'boom'))
    expect(r.title).toBe(MODELS.errInternal)
    expect(r.detail).toMatch(/internal_error/)
  })
})
```

- [ ] **步骤 2：运行确认失败**

```bash
cd web/chat && npx vitest run src/strings.models.test.ts
```

预期：FAIL（`MODELS` / `modelErrorText` 未导出）

- [ ] **步骤 3：最少实现**

在 `strings.ts` 增加（文案可微调但测试断言的键值必须一致）：

```ts
export const MODELS = {
  title: '模型',
  description: '管理对话与理解用的模型；可按任务档位区分，并支持「智能选择」。',
  add: '添加模型',
  edit: '编辑',
  delete: '删除',
  save: '保存',
  cancel: '取消',
  emptyTitle: '还没有模型',
  emptyDescAdmin: '添加一个模型后，对话里就能选用。',
  emptyDescOperator: '暂无可用模型，请联系管理员添加。',
  advanced: '高级',
  fieldName: '名称',
  fieldBaseUrl: '服务地址',
  fieldModel: '模型名',
  fieldApiKey: 'API 密钥',
  fieldApiKeyEnv: 'API Key 环境变量名',
  fieldTier: '任务档位',
  fieldVision: '视觉（支持图片附件）',
  fieldDisableThinking: '禁用思考',
  fieldContextTokens: '上下文长度',
  confirmDeleteTitle: '删除这个模型？',
  confirmDeleteBody: '删除后不可恢复。',
  confirmDeleteLast: '这是当前唯一的模型，删除后对话将无法选择模型。',
  confirmDeleteOk: '删除',
  toastSaved: '已保存模型',
  toastDeleted: '已删除模型',
  errGeneric: '操作未能完成，请稍后重试。',
  errNotFound: '找不到该模型，可能已被删除。',
  errInvalidRequest: '填写内容不完整或不正确，请检查后重试。',
  errInternal: '服务暂时出了问题，请稍后重试。',
  errNameRequired: '请填写名称。',
  errBaseUrlRequired: '请填写服务地址。',
  errModelRequired: '请填写模型名。',
} as const

export function modelErrorText(e: unknown): FriendlyError {
  if (e instanceof ApiError) {
    if (e.code === 'not_found') return { title: MODELS.errNotFound }
    if (e.code === 'invalid_request') return { title: MODELS.errInvalidRequest }
    if (e.code === 'internal_error' || e.status >= 500) {
      return { title: MODELS.errInternal, detail: `${e.code}: ${e.message}` }
    }
    return { title: MODELS.errGeneric, detail: `${e.code}: ${e.message}` }
  }
  return friendlyError(e)
}
```

同步：`buildCreatePayload` / `buildPatchPayload` 内中文校验消息改为引用 `MODELS.errNameRequired` 等（任务 2 改页面时一并改亦可；本任务至少导出常量）。

- [ ] **步骤 4：运行确认通过**

```bash
cd web/chat && npx vitest run src/strings.models.test.ts
```

- [ ] **步骤 5：Commit**

```bash
git add web/chat/src/strings.ts web/chat/src/strings.models.test.ts
git commit -m "feat(web): 模型设置页文案与人话错误映射"
```

---

### 任务 2：模型列表壳 — PageHeader / EmptyState / 按钮去重 / Toast

把 `ModelSettings` 页头与列表反馈改为原语；**尚不**改编辑为 Modal（任务 3）、**尚不**改删除 ConfirmDialog（任务 4）。本任务后：无模型时仅 EmptyState 有「添加模型」；有模型时 PageHeader 有添加；加载/错误用 Toast 或 muted，禁止裸 `apiErrorMessage` 上屏成功路径。

**文件：**
- 修改：`web/chat/src/pages/ModelSettings.tsx`
- 修改：`web/chat/src/pages/ModelSettings.test.tsx`
- 修改：`web/chat/src/pages/ReadOnlyGate.model-runtime.test.tsx`（标题断言改为 `MODELS.title`）

- [ ] **步骤 1：编写/更新失败测试**

在 `ModelSettings.test.tsx` 增加（用 `react-dom/client` + jsdom，参考 `IdentitiesSettings.test.tsx` / `ConnectorShell.test.tsx`）：

```tsx
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import { MODELS } from '../strings'
import { ModelSettings } from './ModelSettings'

// stub AuthContext / role = admin if page reads role from context — mirror ReadOnlyGate setup

describe('ModelSettings empty header dedupe', () => {
  it('shows add only on EmptyState when list empty', async () => {
    vi.spyOn(api, 'listModelProfiles').mockResolvedValue([])
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    await act(async () => {
      root.render(<ModelSettings />)
    })
    await act(async () => {})
    const addButtons = [...host.querySelectorAll('button')].filter((b) =>
      b.textContent?.includes(MODELS.add),
    )
    expect(addButtons).toHaveLength(1)
    root.unmount()
    host.remove()
  })
})
```

（若页面依赖 `useAuth` / Router，测试里包上与 `ReadOnlyGate.model-runtime.test.tsx` 相同的 provider。）

- [ ] **步骤 2：运行确认失败或旧行为双按钮**

```bash
cd web/chat && npx vitest run src/pages/ModelSettings.test.tsx
```

- [ ] **步骤 3：实现列表壳**

要点：

```tsx
import { Cpu } from 'lucide-react'
import {
  Badge, Button, Card, EmptyState, PageHeader, ToastRegion, useToast,
} from '../components/ui'
import { MODELS, modelErrorText } from '../strings'

const showEmpty = !loading && profiles.length === 0 && !loadFailed
// PageHeader actions={showEmpty ? undefined : (readOnly ? undefined : <Button onClick={openCreate}>{MODELS.add}</Button>)}
// EmptyState action={readOnly ? undefined : <Button onClick={openCreate}>{MODELS.add}</Button>}
// catch: push({ tone:'error', title: modelErrorText(e).title, detail: modelErrorText(e).detail })
```

保留现有 inline 编辑表单暂时可用（任务 3 再迁 Modal）；`openCreate` 可先 `setEditingId('__new__')` 或等价现有状态。

- [ ] **步骤 4：测试通过 + ReadOnlyGate 更新**

```bash
cd web/chat && npx vitest run src/pages/ModelSettings.test.tsx src/pages/ReadOnlyGate.model-runtime.test.tsx
```

- [ ] **步骤 5：Commit**

```bash
git add web/chat/src/pages/ModelSettings.tsx web/chat/src/pages/ModelSettings.test.tsx web/chat/src/pages/ReadOnlyGate.model-runtime.test.tsx
git commit -m "feat(web): 模型页列表壳迁入 PageHeader/EmptyState/Toast"
```

---

### 任务 3：模型新建/编辑 Modal + 高级折叠

**文件：**
- 修改：`web/chat/src/pages/ModelSettings.tsx`（或抽出 `ModelProfileEditorModal.tsx`）
- 修改：`web/chat/src/styles/components.css` — 增加：

```css
.settings-advanced {
  margin-top: var(--space-3);
  border-top: 1px solid var(--border-subtle);
  padding-top: var(--space-3);
}
.settings-advanced > summary {
  cursor: pointer;
  color: var(--text-muted);
  font-size: var(--font-size-sm);
}
```

- 测试：`ModelSettings.test.tsx`

- [ ] **步骤 1：失败测试 — Modal 主区可见、高级默认收起**

```tsx
it('opens create modal with main fields; advanced collapsed by default', async () => {
  vi.spyOn(api, 'listModelProfiles').mockResolvedValue([])
  // render ModelSettings, click MODELS.add
  expect(host.querySelector('[role="dialog"]')).toBeTruthy()
  expect(host.textContent).toContain(MODELS.fieldBaseUrl)
  expect(host.textContent).toContain(MODELS.fieldTier)
  const details = host.querySelector('details.settings-advanced')
  expect(details).toBeTruthy()
  expect(details?.open).toBe(false)
  // 高级内字段在 DOM 中但 details 未 open；点击 summary 后可见禁用思考文案
})
```

另测：保存调用 `createModelProfile` / `updateModelProfile`，成功后 dialog 关闭且 Toast（可断言 `MODELS.toastSaved` 出现）。

- [ ] **步骤 2：运行确认失败**

```bash
cd web/chat && npx vitest run src/pages/ModelSettings.test.tsx -t "create modal"
```

- [ ] **步骤 3：实现 Modal**

```tsx
<Modal open={modalOpen} title={editing ? MODELS.edit : MODELS.add} onClose={busy ? undefined : close}
  footer={<>
    <Button variant="ghost" disabled={busy} onClick={close}>{MODELS.cancel}</Button>
    <Button disabled={busy} onClick={() => void save()}>{MODELS.save}</Button>
  </>}>
  <Field label={MODELS.fieldName}><Input ... /></Field>
  <Field label={MODELS.fieldBaseUrl}><Input ... /></Field>
  <Field label={MODELS.fieldModel}><Input ... /></Field>
  <Field label={MODELS.fieldApiKey}><Input type="password" ... /></Field>
  <Field label={MODELS.fieldTier}><Select options={TIER_OPTIONS} ... /></Field>
  <details className="settings-advanced">
    <summary>{MODELS.advanced}</summary>
    {/* vision checkbox, disableThinking, contextTokens, apiKeyEnv */}
  </details>
  {formError && <p className="field-error">{formError}</p>}
</Modal>
```

删除页内联 `ModelProfileForm` 的常显编辑区；`buildCreatePayload` / `buildPatchPayload` 校验失败时 `setFormError(message)`，成功路径用 `modelErrorText` 处理 API 错。

- [ ] **步骤 4：全相关测试通过**

```bash
cd web/chat && npx vitest run src/pages/ModelSettings.test.tsx src/pages/ReadOnlyGate.model-runtime.test.tsx
npx tsc --noEmit
```

- [ ] **步骤 5：Commit**

```bash
git add web/chat/src/pages/ModelSettings.tsx web/chat/src/pages/ModelSettings.test.tsx web/chat/src/styles/components.css
git commit -m "feat(web): 模型新建编辑改为 Modal 并折叠高级字段"
```

---

### 任务 4：模型删除 ConfirmDialog + D1 收尾

**文件：** `ModelSettings.tsx`、`ModelSettings.test.tsx`

- [ ] **步骤 1：失败测试**

```tsx
it('asks ConfirmDialog before delete; last model shows extra warning', async () => {
  vi.spyOn(api, 'listModelProfiles').mockResolvedValue([onlyProfile])
  vi.spyOn(api, 'deleteModelProfile').mockResolvedValue()
  // click delete → expect dialog with MODELS.confirmDeleteTitle and confirmDeleteLast
  // confirm → expect deleteModelProfile called
})
```

断言：页面内 **无** `window.confirm` 调用（可 `vi.spyOn(window, 'confirm')` expect not called）。

- [ ] **步骤 2：运行失败**

- [ ] **步骤 3：实现**

```tsx
<ConfirmDialog
  open={!!pendingDelete}
  title={MODELS.confirmDeleteTitle}
  body={isLast ? `${MODELS.confirmDeleteBody}\n${MODELS.confirmDeleteLast}` : MODELS.confirmDeleteBody}
  confirmText={MODELS.confirmDeleteOk}
  busy={busy}
  error={deleteError}
  onCancel={busy ? undefined : () => setPendingDelete(null)}
  onConfirm={() => void confirmDelete()}
/>
```

- [ ] **步骤 4：测试 + build**

```bash
cd web/chat && npx vitest run src/pages/ModelSettings.test.tsx src/strings.models.test.ts
npm run build
# 按仓库既有方式同步 internal/ui/dist（与 P3-C 相同脚本/习惯）
```

- [ ] **步骤 5：Commit（含 dist）**

```bash
git add web/chat/src/pages/ModelSettings.tsx web/chat/src/pages/ModelSettings.test.tsx internal/ui/dist
git commit -m "feat(web): 模型删除改用 ConfirmDialog 并重建嵌入资源"
```

---

## 批次 D2：技能

### 任务 5：`SKILLS` + `skillErrorText`

**文件：** `strings.ts`、`strings.skills.test.ts`

- [ ] **步骤 1：失败测试**

```ts
import { ApiError } from './api'
import { SKILLS, skillErrorText } from './strings'

expect(SKILLS.title).toBe('技能')
expect(skillErrorText(new ApiError(404, 'not_found', 'x')).title).toBe(SKILLS.errNotFound)
```

常量至少含：`title`、`description`、`upload`、`saveDefaults`、`emptyTitle`、`emptyDesc`、`confirmDeleteTitle`、`confirmDeleteBody`、`confirmDeleteOk`、`toastUploaded`、`toastSaved`、`toastDeleted`、`sourceBuiltin`、`sourceUser`、`err*`。

- [ ] **步骤 2–5：** 实现 → 测试通过 → commit `feat(web): 技能设置页文案与人话错误映射`

---

### 任务 6：技能页原语化 + ConfirmDialog + Toast

**文件：** `SkillsSettings.tsx`；新增 `SkillsSettings.test.tsx`（交互）；更新 `ReadOnlyGate.tools-skills.test.tsx`（标题「技能」）。

- [ ] **步骤 1：失败测试**

```tsx
it('uses humanized title and ConfirmDialog on user skill delete', async () => {
  // mock listSkills / getAgent
  expect(host.textContent).toContain(SKILLS.title)
  expect(host.textContent).not.toMatch(/\bSkills\b/)
  // click delete on source=user → dialog → confirm → deleteSkill called
})
it('disables checkboxes for operator', async () => { /* mirror ReadOnlyGate */ })
```

- [ ] **步骤 2：运行失败**

- [ ] **步骤 3：实现**

- `PageHeader` + `ToastRegion`；去掉主视觉「默认 Agent：{id}」（可放 `settings-muted` 次要一行或删除）。
- 上传 / 保存 / 删除：`skillErrorText` + Toast；删除走 `ConfirmDialog`。
- 列表行用 `Card` 或保留 list 结构但按钮换 `Button`；来源 Badge 用 `SKILLS.sourceBuiltin` / `sourceUser`。
- EmptyState 无技能时引导上传（admin）。

- [ ] **步骤 4：测试 + tsc**

```bash
cd web/chat && npx vitest run src/pages/SkillsSettings.test.tsx src/pages/SkillsSettings.test.ts src/pages/ReadOnlyGate.tools-skills.test.tsx src/strings.skills.test.ts
npx tsc --noEmit
```

- [ ] **步骤 5：build dist + commit**

```bash
npm run build
# sync internal/ui/dist
git commit -m "feat(web): 技能页人话化并统一确认框与 Toast"
```

---

## 批次 D3：助手功能

### 任务 7：`TOOLS` + `toolErrorText`

**文件：** `strings.ts`、`strings.tools.test.ts`

- [ ] **步骤 1：失败测试** — `TOOLS.title === '助手功能'`；捕获字段标签：

```ts
expect(TOOLS.captureToolGlob).toBe('匹配哪些登录功能')
expect(TOOLS.captureTokenPaths).toBe('令牌字段路径（每行一条）')
expect(TOOLS.captureLabelPaths).toBe('显示名字段路径（每行一条）')
expect(TOOLS.captureHeaderTemplate).toBe('请求头模板')
expect(TOOLS.captureDefaultScheme).toBe('默认认证方案')
expect(TOOLS.advanced).toBe('高级')
expect(TOOLS.addTool).toBe('添加')
expect(TOOLS.viewSchema).toBe('查看参数说明')
```

`toolErrorText` 覆盖 `not_found` / `invalid_request` / `internal_error`。

- [ ] **步骤 2–5：** 实现 → 测试 → commit `feat(web): 助手功能页文案与人话错误映射`

---

### 任务 8：助手功能页壳 — 标题、树保留、Toast、删除 ConfirmDialog、schema 折叠

**文件：** `ToolsSettings.tsx`；新增/扩展 `ToolsSettings.test.tsx`；`ReadOnlyGate.tools-skills.test.tsx`。

- [ ] **步骤 1：失败测试**

```tsx
it('renders 助手功能 not Tools', async () => {
  expect(host.textContent).toContain(TOOLS.title)
  expect(host.querySelector('h1')?.textContent).not.toBe('Tools')
})
it('confirms before deleteConnectorTool', async () => {
  vi.spyOn(api, 'deleteConnectorTool').mockResolvedValue()
  // trigger delete on extra tool → ConfirmDialog → confirm → called
})
```

- [ ] **步骤 2：运行失败**

- [ ] **步骤 3：实现（本任务仍可用抽屉添加；任务 9 改 Modal）**

- 页头 `PageHeader` + 搜索 + admin 添加按钮。
- 所有 `errorMessage` → `toolErrorText` + Toast（或分区错误改 Toast）。
- 删除 extra：`ConfirmDialog`。
- schema：`<details><summary>{TOOLS.viewSchema}</summary><pre>…</pre></details>`。
- 按钮换 `Button`；尽量用令牌 class，树结构 CSS（`settings-tree` 等）可暂留以免大爆炸，但标题/错误/确认必须达标。

- [ ] **步骤 4：测试通过**

- [ ] **步骤 5：Commit** `feat(web): 助手功能页标题人话化并补删除确认`

---

### 任务 9：登录捕获人话化 + 默认收进高级 + merge 回归

**文件：** `CaptureSettingsFields.tsx`、`ToolsSettings.tsx`、测试。

- [ ] **步骤 1：失败测试**

```tsx
describe('CaptureSettingsFields', () => {
  it('uses humanized labels and starts collapsed in parent advanced details', () => {
    // render connector settings strip with capture
    const adv = host.querySelector('details.settings-advanced')
    expect(adv && !adv.open).toBe(true)
    expect(host.textContent).toContain(TOOLS.captureToolGlob)
    expect(host.textContent).not.toContain('token_json_paths')
  })
})

describe('saveConnectorSettings capture merge', () => {
  it('PUT body includes mergeAuthWithCapture result', async () => {
    const put = vi.spyOn(api, 'putConnector').mockResolvedValue(meta)
    // edit glob in draft, click save
    expect(put.mock.calls[0][1].auth?.capture?.tool_name_glob).toBe('*login*')
  })
})
```

- [ ] **步骤 2：运行失败**

- [ ] **步骤 3：实现**

`CaptureSettingsFields`：全部标签改 `TOOLS.capture*`；hint 人话化（去掉「侧车/glob/operation」堆砌，改短说明）。父级用：

```tsx
<details className="settings-advanced">
  <summary>{TOOLS.advanced}</summary>
  <CaptureSettingsFields ... />
</details>
```

`saveConnectorSettings` **必须继续** `mergeAuthWithCapture(meta.auth, captureDraft)`；失败 Toast 用 `toolErrorText`。

- [ ] **步骤 4：测试通过**

- [ ] **步骤 5：Commit** `feat(web): 登录捕获字段人话化并默认折叠高级`

---

### 任务 10：添加工具改 Modal + D3 收尾

**文件：** `ToolsSettings.tsx`、测试、`components.css`（若需 modal 内表单间距）、`internal/ui/dist`。

- [ ] **步骤 1：失败测试**

```tsx
it('opens add-tool Modal instead of drawer', async () => {
  // click TOOLS.addTool
  expect(host.querySelector('.settings-drawer')).toBeNull()
  expect(host.querySelector('[role="dialog"]')).toBeTruthy()
})
```

- [ ] **步骤 2：运行失败**

- [ ] **步骤 3：实现** — 将抽屉表单字段迁入 `Modal`；字段标签人话（所属连接、请求方法、路径、参数说明）；提交成功 Toast + 关闭 + 刷新；`drawerError` → Modal 内错误或 Toast。

- [ ] **步骤 4：全量前端门禁**

```bash
cd web/chat && npm test && npx tsc --noEmit && npm run build
# sync internal/ui/dist
```

- [ ] **步骤 5：Commit** `feat(web): 助手功能添加工具改用 Modal 并重建嵌入资源`

---

## 自检（对照规格）

| 规格条目 | 任务 |
|----------|------|
| §4 共用约定（原语/Toast/Confirm/文案/空列表去重/readOnly） | 2–4, 6, 8–10 |
| §5 D1 Modal + 高级 + 删除确认 | 3, 4 |
| §6 D2 标题/Confirm/Toast | 5, 6 |
| §7 D3 树保留、Modal 添加、捕获高级、merge | 8–10 |
| §8 MODELS/SKILLS/TOOLS + 三分错误函数 | 1, 5, 7 |
| §9 测试与 dist | 各批末尾 |
| §10 非目标 | 无任务触碰后端/消息组/OAuth/@ 直达 |

占位符扫描：无 TODO/待定。类型名：`MODELS`/`SKILLS`/`TOOLS`、`modelErrorText`/`skillErrorText`/`toolErrorText`、`mergeAuthWithCapture` 与现码一致。

---

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-09-12-webui-refresh-p3d-assistant-pages.md`。

**两种执行方式：**

1. **子代理驱动（推荐）** — 每个任务调度一个新子代理，任务间审查，快速迭代  
2. **内联执行** — 当前会话用 executing-plans，批量执行并设检查点  

选哪种方式？
