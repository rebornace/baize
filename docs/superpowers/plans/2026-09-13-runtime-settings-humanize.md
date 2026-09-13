# 运行参数页人话化实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将「运行参数」页迁到 P3 原语 + 三分区人话壳；压缩细参默认折叠；口令区温和人话；文案集中 `RUNTIME`；不改后端。

**架构：** 以前端为主。`RUNTIME` 为文案唯一来源；`runtimeSettingsHelpers` 导出主区/压缩高级字段列表（`key` + 范围元数据，label/hint 读 `RUNTIME`）；`RuntimeSettings` 用 `PageHeader`/`Field`/`Input`/`Button`/`Badge`/`Toast`/`ConfirmDialog`；引擎参数一次保存；口令区独立表单。不改 Go。

**技术栈：** React 19、TypeScript、Vite 6、Vitest + jsdom、`web/chat`。命令：`cd web/chat && npm test`、`npx tsc --noEmit`、`npm run build`（→ `internal/ui/dist`）。提交中文 Conventional Commits。

规格：`docs/superpowers/specs/2026-09-13-runtime-settings-humanize-design.md`

对照实现：`WebhookSettings.tsx`（PageHeader + useToast + Field）、`RuntimeSettings.tsx`（现逻辑）、`RuntimeSettings.test.tsx`（ConfirmDialog）。

---

## 文件结构

修改：

- `web/chat/src/strings.ts` — 扩写 `RUNTIME`（页头、分区、字段、折叠、Toast、校验、口令）
- `web/chat/src/pages/runtimeSettingsHelpers.ts` — `MAIN_KNOB_FIELDS` / `COMPACT_ADV_FIELDS`；校验文案用人话；可选 `allKnobFieldSpecs()` 供遍历校验
- `web/chat/src/pages/runtimeSettingsHelpers.test.ts` — 分组与校验断言
- `web/chat/src/pages/RuntimeSettings.tsx` — 人话壳 + 三分区 + 压缩折叠 + Toast + 口令原语
- `web/chat/src/pages/RuntimeSettings.test.tsx` — 扩：标题、分区、折叠默认收起、重置按钮文案
- `web/chat/src/pages/ReadOnlyGate.model-runtime.test.tsx` — 断言新人话标题/分区；无「控制面口令」
- `internal/ui/dist/**` — 末任务重建
- `docs/superpowers/specs/2026-09-13-runtime-settings-humanize-design.md` — 状态改为已交付
- `docs/superpowers/notes/2026-09-13-spec-ledger.md` — UI-RUNTIME 行改为已交付

不改：Go API、ACL、路由、`settingsNav` 的 `to`（label 已是「运行参数」）。

---

## 任务 1：扩写 `RUNTIME` + helpers 字段分组

**文件：**
- 修改：`web/chat/src/strings.ts`
- 修改：`web/chat/src/pages/runtimeSettingsHelpers.ts`
- 修改：`web/chat/src/pages/runtimeSettingsHelpers.test.ts`

- [ ] **步骤 1：先改 helpers 测试（期望新分组 API）**

将 `runtimeSettingsHelpers.test.ts` 中依赖 `KNOB_FIELDS[0]` / `[1]` / `[3]` 的用例改为按 `key` 查找，并增加分组测试：

```ts
import {
  buildKnobsPatch,
  knobsToForm,
  MAIN_KNOB_FIELDS,
  COMPACT_ADV_FIELDS,
  allKnobFieldSpecs,
  validateKnobField,
  type KnobsForm,
} from './runtimeSettingsHelpers'
import { RUNTIME } from '../strings'

// ... baseKnobs 不变 ...

describe('field groups', () => {
  it('splits main vs compact advanced keys', () => {
    expect(MAIN_KNOB_FIELDS.map((f) => f.key)).toEqual([
      'max_messages',
      'max_steps',
      'tool_timeout_seconds',
    ])
    expect(COMPACT_ADV_FIELDS.map((f) => f.key)).toEqual([
      'compact_threshold',
      'compact_reserve_tokens',
      'compact_keep_recent',
      'compact_summary_timeout_seconds',
    ])
  })

  it('allKnobFieldSpecs covers seven numeric fields', () => {
    expect(allKnobFieldSpecs()).toHaveLength(7)
  })
})

describe('validateKnobField', () => {
  const maxMessages = MAIN_KNOB_FIELDS.find((f) => f.key === 'max_messages')!
  const maxSteps = MAIN_KNOB_FIELDS.find((f) => f.key === 'max_steps')!
  const threshold = COMPACT_ADV_FIELDS.find((f) => f.key === 'compact_threshold')!

  it('accepts in-range values', () => {
    expect(validateKnobField(maxMessages, '100')).toBeNull()
    expect(validateKnobField(threshold, '0.5')).toBeNull()
  })

  it('rejects out-of-range with human label from RUNTIME', () => {
    const msg = validateKnobField(maxSteps, '0')
    expect(msg).toContain(RUNTIME.fieldMaxSteps)
  })

  // 其余 reject 用例同理用 find-by-key；保留 buildKnobsPatch 套件不变
})
```

删除对已废弃导出名 `KNOB_FIELDS` 的导入（若仍导出作别名则可暂时保留，但测试只用新名）。

- [ ] **步骤 2：运行确认失败**

```powershell
cd web/chat
npm test -- src/pages/runtimeSettingsHelpers.test.ts
```

预期：FAIL（无 `MAIN_KNOB_FIELDS` / `RUNTIME.fieldMaxSteps` 等）。

- [ ] **步骤 3：实现 `RUNTIME` 扩展与 helpers**

在 `strings.ts` 将现有 `RUNTIME` 扩为（键名稳定；可微调句式但测试依赖的键必须存在）：

```ts
export const RUNTIME = {
  title: '运行参数',
  descriptionAdmin:
    '调整对话长度、工具超时与历史压缩；保存后立即生效，无需重启。控制面口令也可在本页轮换。',
  descriptionOperator: '只读查看当前生效的运行参数；修改请联系管理员。',
  sectionBehavior: '对话与工具行为',
  sectionCompact: '历史压缩',
  sectionCreds: '控制面口令',
  compactEnabled: '对话过长时自动压缩历史',
  compactHint: '压缩会把旧消息收成摘要以省上下文；细参多数保持默认即可。',
  compactAdvanced: '高级压缩设置',
  fieldMaxMessages: '送入模型的最近消息数',
  hintMaxMessages: 'max_messages · 1–500',
  fieldMaxSteps: '单次运行最多工具步数',
  hintMaxSteps: 'max_steps · 1–100',
  fieldToolTimeout: '单个工具最长等待（秒）',
  hintToolTimeout: 'tool_timeout_seconds · 1–600',
  fieldCompactThreshold: '触发压缩的上下文占用比例',
  hintCompactThreshold: 'compact_threshold · 0.1–0.95',
  fieldCompactReserve: '压缩时预留的 token 数',
  hintCompactReserve: 'compact_reserve_tokens · 256–100000',
  fieldCompactKeep: '压缩后保留的最近原文条数',
  hintCompactKeep: 'compact_keep_recent · 0–100',
  fieldCompactSummaryTimeout: '生成摘要的最长等待（秒）',
  hintCompactSummaryTimeout: 'compact_summary_timeout_seconds · 1–600',
  badgeOverridden: '已覆盖基线',
  saveKnobs: '保存运行参数',
  saving: '保存中…',
  toastKnobsSaved: '运行参数已保存，下一次运行立即生效',
  toastNoChange: '没有改动',
  loadFailed: '无法加载运行参数',
  errMustNumber: '必须是数字',
  errMustInt: '必须是整数',
  errOutOfRange: '超出允许范围',
  // 口令区
  credsSourceOverride: '热更新覆盖',
  credsSourceConfig: '配置基线',
  credsOperatorSet: '运营口令已设置',
  credsOperatorUnset: '运营口令未设置',
  credsAdminSet: '管理口令已设置',
  credsAdminUnset: '管理口令未设置',
  rotateTitle: '轮换主口令',
  fieldOperatorToken: '运营口令',
  hintOperatorToken: 'operator · 留空表示不修改',
  fieldAdminToken: '管理口令',
  hintAdminToken: 'admin · 留空表示不修改',
  rotateSubmit: '轮换主口令',
  rotateNeedOne: '请至少填写一个要轮换的口令',
  toastRotated: '口令已轮换；若改的是当前登录口令，请用新口令重新解锁。',
  namedOpsTitle: '命名运营账号',
  namedOpsEmpty: '暂无命名运营账号。',
  fieldNewOpId: '账号 id',
  fieldNewOpToken: '口令',
  addOperator: '新增账号',
  addNeedBoth: '新增账号需要填写 id 与口令',
  removeOperator: '移除',
  toastAdded: '已新增运营账号',
  toastRemoved: '已移除运营账号',
  resetButton: '重置为配置基线口令',
  confirmResetTitle: '重置为配置基线口令？',
  confirmResetBody: '将清空全部热更新凭据，回落到 YAML/环境变量中的基线口令。引擎参数不受影响。',
  confirmResetOk: '重置',
  toastReset: '已重置：口令回落至配置基线（引擎参数不受影响）。',
  badgeRuntime: '热更新',
  badgeConfig: '配置',
} as const
```

`runtimeSettingsHelpers.ts` 形状：

```ts
import { RUNTIME } from '../strings'

export interface KnobFieldSpec {
  key: keyof Omit<KnobsForm, 'compaction_enabled'>
  label: string
  hint: string
  min: number
  max: number
  integer: boolean
}

export const MAIN_KNOB_FIELDS: KnobFieldSpec[] = [
  { key: 'max_messages', label: RUNTIME.fieldMaxMessages, hint: RUNTIME.hintMaxMessages, min: 1, max: 500, integer: true },
  { key: 'max_steps', label: RUNTIME.fieldMaxSteps, hint: RUNTIME.hintMaxSteps, min: 1, max: 100, integer: true },
  { key: 'tool_timeout_seconds', label: RUNTIME.fieldToolTimeout, hint: RUNTIME.hintToolTimeout, min: 1, max: 600, integer: true },
]

export const COMPACT_ADV_FIELDS: KnobFieldSpec[] = [
  { key: 'compact_threshold', label: RUNTIME.fieldCompactThreshold, hint: RUNTIME.hintCompactThreshold, min: 0.1, max: 0.95, integer: false },
  { key: 'compact_reserve_tokens', label: RUNTIME.fieldCompactReserve, hint: RUNTIME.hintCompactReserve, min: 256, max: 100000, integer: true },
  { key: 'compact_keep_recent', label: RUNTIME.fieldCompactKeep, hint: RUNTIME.hintCompactKeep, min: 0, max: 100, integer: true },
  { key: 'compact_summary_timeout_seconds', label: RUNTIME.fieldCompactSummaryTimeout, hint: RUNTIME.hintCompactSummaryTimeout, min: 1, max: 600, integer: true },
]

export function allKnobFieldSpecs(): KnobFieldSpec[] {
  return [...MAIN_KNOB_FIELDS, ...COMPACT_ADV_FIELDS]
}

/** @deprecated 勿用；保留一版别名以免遗漏引用时可 grep */
export const KNOB_FIELDS = allKnobFieldSpecs()

export function validateKnobField(spec: KnobFieldSpec, raw: string): string | null {
  const v = Number(raw)
  if (raw.trim() === '' || Number.isNaN(v)) {
    return `${spec.label} ${RUNTIME.errMustNumber}`
  }
  if (spec.integer && !Number.isInteger(v)) {
    return `${spec.label} ${RUNTIME.errMustInt}`
  }
  if (v < spec.min || v > spec.max) {
    return `${spec.label} ${RUNTIME.errOutOfRange}（${spec.min}–${spec.max}）`
  }
  return null
}
```

`knobsToForm` / `buildKnobsPatch` 保持不变。提交前 `grep KNOB_FIELDS`：页面改为用 `MAIN_` / `COMPACT_` / `allKnobFieldSpecs()`。

- [ ] **步骤 4：运行确认通过**

```powershell
cd web/chat
npm test -- src/pages/runtimeSettingsHelpers.test.ts
```

预期：PASS。

- [ ] **步骤 5：Commit**

```powershell
git add web/chat/src/strings.ts web/chat/src/pages/runtimeSettingsHelpers.ts web/chat/src/pages/runtimeSettingsHelpers.test.ts
git commit -m "feat(web): 运行参数文案与字段分组（MAIN/COMPACT）"
```

---

## 任务 2：`RuntimeSettings` 引擎参数区人话壳

**文件：**
- 修改：`web/chat/src/pages/RuntimeSettings.tsx`
- 修改：`web/chat/src/pages/RuntimeSettings.test.tsx`
- 修改：`web/chat/src/pages/ReadOnlyGate.model-runtime.test.tsx`

- [ ] **步骤 1：扩展失败测试（标题 / 分区 / 折叠）**

在 `RuntimeSettings.test.tsx` 增加：

```ts
describe('RuntimeSettings humanize shell', () => {
  afterEach(() => { vi.restoreAllMocks() })

  it('shows PageHeader title and three section headings; compact adv collapsed', async () => {
    const { host, root } = await renderRuntime()
    expect(host.textContent).toContain(RUNTIME.title)
    expect(host.textContent).not.toContain('运行时设置')
    expect(host.textContent).toContain(RUNTIME.sectionBehavior)
    expect(host.textContent).toContain(RUNTIME.sectionCompact)
    expect(host.textContent).toContain(RUNTIME.sectionCreds)
    // 高级区内字段默认不可见：details 未 open，或不在 DOM 可见区
    const details = host.querySelector('details')
    expect(details).toBeTruthy()
    expect(details!.open).toBe(false)
    expect(host.textContent).toContain(RUNTIME.fieldMaxMessages)
    // 折叠未开时，高级 label 仍可能在 summary 旁；字段 input 应在 details 内
    const advInputs = details!.querySelectorAll('input')
    expect(advInputs.length).toBeGreaterThanOrEqual(4)
    root.unmount()
    host.remove()
  })
})
```

更新 ConfirmDialog 用例中按钮文案匹配 `RUNTIME.resetButton`（「重置为配置基线口令」）。

`ReadOnlyGate.model-runtime.test.tsx` 中运营用例改为：

```ts
expect(host.textContent).toContain(RUNTIME.title)
expect(host.textContent).toContain(RUNTIME.sectionBehavior)
expect(host.textContent).not.toContain(RUNTIME.saveKnobs)
expect(host.textContent).not.toContain(RUNTIME.sectionCreds)
expect(host.textContent).not.toContain(RUNTIME.rotateSubmit)
```

并 `import { RUNTIME } from '../strings'`。运营 mock 的 runtime 响应需带齐 `effective` 全字段（与 `baseKnobs` 同形），避免表单崩。

- [ ] **步骤 2：运行确认失败**

```powershell
cd web/chat
npm test -- src/pages/RuntimeSettings.test.tsx src/pages/ReadOnlyGate.model-runtime.test.tsx
```

预期：FAIL（仍有「运行时设置」/「引擎参数」等）。

- [ ] **步骤 3：改写 `RuntimeSettings` 引擎区**

要点（口令区可暂留旧壳，任务 3 再换，但区标题先用 `RUNTIME.sectionCreds` 以免只读测失败）：

```tsx
import {
  Badge, Button, Field, Input, PageHeader, ToastRegion, useToast,
} from '../components/ui'
import { RUNTIME, friendlyError } from '../strings'
import {
  MAIN_KNOB_FIELDS, COMPACT_ADV_FIELDS, allKnobFieldSpecs, ...
} from './runtimeSettingsHelpers'

// load/save：错误用 friendlyError + push toast；成功 toastKnobsSaved / toastNoChange
// 校验：for (const spec of allKnobFieldSpecs())

return (
  <div className="settings-section">
    <PageHeader
      title={RUNTIME.title}
      description={readOnly ? RUNTIME.descriptionOperator : RUNTIME.descriptionAdmin}
    />
    <ToastRegion toasts={toasts} onDismiss={dismiss} />
    {/* loading / 错误可用 toast 或短文案 */}
    <form onSubmit={...}>
      <h2>{RUNTIME.sectionBehavior}</h2>
      {MAIN_KNOB_FIELDS.map((spec) => (
        <Field key={spec.key} label={<>{spec.label}{overridden && <Badge>{RUNTIME.badgeOverridden}</Badge>}</>} hint={spec.hint}>
          <Input type="number" ... disabled={busy || readOnly} />
        </Field>
      ))}
      <h2>{RUNTIME.sectionCompact}</h2>
      <p>{RUNTIME.compactHint}</p>
      <label>
        <input type="checkbox" checked={form.compaction_enabled} ... />
        {RUNTIME.compactEnabled}
      </label>
      <details>
        <summary>{RUNTIME.compactAdvanced}</summary>
        {COMPACT_ADV_FIELDS.map(... Field+Input ...)}
      </details>
      {!readOnly && <Button type="submit" variant="primary" disabled={busy}>{busy ? RUNTIME.saving : RUNTIME.saveKnobs}</Button>}
    </form>
    <CredentialsSection ... />
  </div>
)
```

`Field` 的 `label` 若只接受 `string`，则徽章放 label 旁另起 `span`（看 `Field.tsx` props；必要时 label 用字符串、徽章作 sibling）。

对照 `WebhookSettings.tsx` 的 `useToast` / `ToastRegion` 用法。

- [ ] **步骤 4：运行确认通过**

```powershell
cd web/chat
npm test -- src/pages/RuntimeSettings.test.tsx src/pages/ReadOnlyGate.model-runtime.test.tsx
npx tsc --noEmit
```

预期：PASS。

- [ ] **步骤 5：Commit**

```powershell
git add web/chat/src/pages/RuntimeSettings.tsx web/chat/src/pages/RuntimeSettings.test.tsx web/chat/src/pages/ReadOnlyGate.model-runtime.test.tsx
git commit -m "feat(web): 运行参数页引擎区人话壳与压缩折叠"
```

---

## 任务 3：控制面口令区人话 + 原语

**文件：**
- 修改：`web/chat/src/pages/RuntimeSettings.tsx`（`CredentialsSection`）
- 修改：`web/chat/src/pages/RuntimeSettings.test.tsx`

- [ ] **步骤 1：更新重置用例文案断言**

确保按钮与 Dialog 使用 `RUNTIME.resetButton` / `confirmResetTitle` / `confirmResetOk`。可增一例：来源行含 `RUNTIME.credsSourceOverride`。

- [ ] **步骤 2：运行确认（若文案已在任务 2 部分对齐则应仍绿；故意先改测试期望新 toast 键）**

```powershell
cd web/chat
npm test -- src/pages/RuntimeSettings.test.tsx
```

- [ ] **步骤 3：重写 CredentialsSection**

- 区标题 `RUNTIME.sectionCreds`
- `Field` + `Input type="password"` 绑定运营/管理口令
- `Button` 提交轮换 / 新增；移除用 `Button` variant ghost/danger
- 成功/失败 `push` Toast；去掉页内 `status` 段落（或仅保留 loading）
- `ConfirmDialog` 文案已用 `RUNTIME.confirmReset*`
- 命名列表空态 `RUNTIME.namedOpsEmpty`
- **逻辑不变**：`patchCredentials` 形状、`role !== 'admin' return null`、hooks 无条件调用

- [ ] **步骤 4：全量相关测试 + tsc**

```powershell
cd web/chat
npm test -- src/pages/RuntimeSettings.test.tsx src/pages/runtimeSettingsHelpers.test.ts src/pages/ReadOnlyGate.model-runtime.test.tsx
npx tsc --noEmit
```

预期：PASS。

- [ ] **步骤 5：Commit**

```powershell
git add web/chat/src/pages/RuntimeSettings.tsx web/chat/src/pages/RuntimeSettings.test.tsx web/chat/src/strings.ts
git commit -m "feat(web): 运行参数口令区温和人话与原语"
```

---

## 任务 4：构建 dist + 规格收口

**文件：**
- 修改：`internal/ui/dist/**`（build 产物）
- 修改：`docs/superpowers/specs/2026-09-13-runtime-settings-humanize-design.md`（状态 → 已交付）
- 修改：`docs/superpowers/notes/2026-09-13-spec-ledger.md`（UI-RUNTIME → 已交付）
- 修改：`docs/superpowers/notes/2026-09-13-v1-product-confirmation-checklist.md`（UI-RUNTIME 备注 → 已交付）

- [ ] **步骤 1：生产构建**

```powershell
cd web/chat
npm run build
```

预期：成功，更新 `internal/ui/dist`。

- [ ] **步骤 2：更新文档状态**

规格头：`状态：已交付（2026-09-13）`。账本/清单对应行改为已交付。

- [ ] **步骤 3：Commit**

```powershell
git add internal/ui/dist docs/superpowers/specs/2026-09-13-runtime-settings-humanize-design.md docs/superpowers/notes/2026-09-13-spec-ledger.md docs/superpowers/notes/2026-09-13-v1-product-confirmation-checklist.md
git commit -m "chore(web): 重建 UI dist；标记运行参数人话化已交付"
```

---

## 自检（对照规格）

| 规格要求 | 任务 |
|----------|------|
| PageHeader / 标题「运行参数」 | 2 |
| 三分区 | 2–3 |
| 压缩开关 + 高级折叠默认收起 | 1–2 |
| 口令温和人话 | 3 |
| Toast / strings / friendlyError | 2–3 |
| 运营只读 | 2（ReadOnlyGate） |
| 无 Go 变更 | 全程 |
| 文案抽离附录 | 1（`RUNTIME` 键） |
| dist + 规格已交付 | 4 |

无占位符；`MAIN_KNOB_FIELDS` / `COMPACT_ADV_FIELDS` / `RUNTIME.*` 命名跨任务一致。

---

*计划结束后请选择：子代理驱动（推荐）或内联执行。*
