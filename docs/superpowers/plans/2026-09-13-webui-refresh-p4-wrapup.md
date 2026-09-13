# WebUI P4 收尾实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 闭合 WebUI 体验改版 P4：抽出 `styles/chat.css`、裸色门禁、Runtime 危险确认改 ConfirmDialog、按规格走查并修中高优问题、README 导航人话对齐、全量绿并重建 `internal/ui/dist`、填写验收 note。

**架构：** 以前端工程与验收为主，不改后端契约。CSS 整文件零行为搬移；用 vitest 锁门禁与 ConfirmDialog；人工走查结果写入私有仓 notes；README 只改用户路径用语。

**技术栈：** React 19、TypeScript、Vite 6、Vitest + jsdom、`web/chat`。命令均在 `web/chat` 下：`npm test`、`npx tsc --noEmit`、`npm run build`（`outDir` → `../../internal/ui/dist`）。提交进 `real/main`，中文 Conventional Commits。开源导出仅在用户下令后执行。

规格：`docs/superpowers/specs/2026-09-13-webui-refresh-p4-wrapup-design.md`

---

## 文件结构

新增：

- `web/chat/src/styles/chat.css` — 自 `style.css` 整文件迁入的聊天样式
- `web/chat/src/styles/noHardcodedColors.test.ts` — 扫描 CSS 禁止裸色（白名单 `tokens.css` + 允许注释/透明渐变）
- `web/chat/src/pages/RuntimeSettings.test.tsx` — ConfirmDialog 替换 `window.confirm` 回归
- `web/chat/src/main.imports.test.ts` — 断言引入 `styles/chat.css` 且不再引入 `./style.css`
- `docs/superpowers/notes/2026-09-13-webui-p4-acceptance.md` — 走查矩阵勾选与修 bug 列表

修改：

- `web/chat/src/main.tsx` — CSS import 路径
- `web/chat/src/pages/RuntimeSettings.tsx` — ConfirmDialog；`strings.ts` 增加确认文案键（若项目惯例要求集中文案）
- `web/chat/src/strings.ts` — Runtime 确认标题/正文/按钮（可选但推荐）
- `README.md`、`README.zh-CN.md` — 设置导航人话（Tools→助手功能、OpenAPI→业务系统、Inbox→外部来信、MCP 导出→对外提供能力、Webhook→消息回调等）
- `docs/superpowers/notes/2026-09-10-webui-p1-chat-visual-acceptance.md` — 待复测项标为已关闭或指向 P4 note
- `internal/ui/dist/**` — 末任务重建
- 走查发现的样式/组件文件（按需：`styles/chat.css`、`settings.css`、`components.css`、相关 tsx）

删除：

- `web/chat/src/style.css` — 迁移并确认无引用后删除

不改：Go API、ACL、路由 id、Playwright、消息组三页全面人话化、F 生产硬化。

---

## 任务 1：CSS 零行为迁移（style.css → styles/chat.css）

**文件：**
- 创建：`web/chat/src/styles/chat.css`
- 创建：`web/chat/src/main.imports.test.ts`
- 修改：`web/chat/src/main.tsx`
- 删除：`web/chat/src/style.css`

- [ ] **步骤 1：编写失败的 import 冒烟测试**

```ts
// web/chat/src/main.imports.test.ts
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const root = join(dirname(fileURLToPath(import.meta.url)))

describe('main CSS imports', () => {
  it('loads styles/chat.css and does not import ./style.css', () => {
    const main = readFileSync(join(root, 'main.tsx'), 'utf8')
    expect(main).toMatch(/import\s+['"]\.\/styles\/chat\.css['"]/)
    expect(main).not.toMatch(/import\s+['"]\.\/style\.css['"]/)
    expect(() => readFileSync(join(root, 'styles', 'chat.css'), 'utf8')).not.toThrow()
  })
})
```

- [ ] **步骤 2：运行测试确认失败**

```powershell
cd web/chat
npm test -- src/main.imports.test.ts
```

预期：FAIL（仍 import `./style.css` 或缺少 `styles/chat.css`）。

- [ ] **步骤 3：整文件迁移并改 import**

1. 将 `src/style.css` 内容原样复制为 `src/styles/chat.css`（不改选择器/属性）。
2. `main.tsx` 中把 `import './style.css'` 改为 `import './styles/chat.css'`。
3. 删除 `src/style.css`。
4. 全仓搜索确认无其它 `style.css` 引用（`web/chat` 内）。

- [ ] **步骤 4：运行测试确认通过 + tsc**

```powershell
cd web/chat
npm test -- src/main.imports.test.ts
npx tsc --noEmit
```

预期：PASS；tsc 零错误。

- [ ] **步骤 5：Commit**

```powershell
git add web/chat/src/styles/chat.css web/chat/src/main.tsx web/chat/src/main.imports.test.ts
git rm web/chat/src/style.css
git commit -m "refactor(web): 将聊天样式迁入 styles/chat.css"
```

---

## 任务 2：裸色门禁

**文件：**
- 创建：`web/chat/src/styles/noHardcodedColors.test.ts`
- 按需修改：`web/chat/src/styles/*.css`（若扫描发现违规，改为令牌）

- [ ] **步骤 1：编写失败/锁定测试**

```ts
// web/chat/src/styles/noHardcodedColors.test.ts
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const stylesDir = dirname(fileURLToPath(import.meta.url))
const ALLOW_FILES = new Set(['tokens.css'])

/** Strip block and line comments so sample hex in comments does not fail the scan. */
function stripComments(src: string): string {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')
}

/** 允许的透明渐变写法（整段匹配后从扫描文本中剔除）。 */
const GRADIENT_ALLOW = [
  /linear-gradient\([^;{}]*transparent[^;{}]*\)/gi,
  /radial-gradient\([^;{}]*transparent[^;{}]*\)/gi,
]

function scanable(src: string): string {
  let s = stripComments(src)
  for (const re of GRADIENT_ALLOW) s = s.replace(re, '')
  return s
}

const COLOR_RE = /#(?:[0-9a-fA-F]{3,8})\b|\brgba?\(/g

describe('no hardcoded colors outside tokens.css', () => {
  it('CSS modules only use tokens (except tokens.css and allowed gradients)', () => {
    const files = readdirSync(stylesDir).filter((f) => f.endsWith('.css'))
    const violations: string[] = []
    for (const file of files) {
      if (ALLOW_FILES.has(file)) continue
      const text = scanable(readFileSync(join(stylesDir, file), 'utf8'))
      const hits = text.match(COLOR_RE)
      if (hits?.length) violations.push(`${file}: ${[...new Set(hits)].join(', ')}`)
    }
    expect(violations, violations.join('\n')).toEqual([])
  })
})
```

- [ ] **步骤 2：运行测试**

```powershell
cd web/chat
npm test -- src/styles/noHardcodedColors.test.ts
```

- [ ] **步骤 3：若 FAIL，消违规**

把报出的裸色改为 `var(--…)` 令牌（必要时在 `tokens.css` 增语义令牌）。透明遮罩/渐变若必需，扩 `GRADIENT_ALLOW` 并在测试旁注释理由。禁止把业务色写回非 tokens 文件。

重跑至 PASS。

- [ ] **步骤 4：Commit**

```powershell
git add web/chat/src/styles/noHardcodedColors.test.ts web/chat/src/styles/
git commit -m "test(web): 禁止样式文件硬编码色值"
```

---

## 任务 3：Runtime 清空凭据改 ConfirmDialog

**文件：**
- 创建：`web/chat/src/pages/RuntimeSettings.test.tsx`
- 修改：`web/chat/src/pages/RuntimeSettings.tsx`
- 修改：`web/chat/src/strings.ts`（新增 `RUNTIME` 确认文案键）

参考现有模式：`ModelSettings.tsx` 的 `pendingDelete` + `ConfirmDialog`；测试模式见 `ModelSettings.test.tsx`「asks ConfirmDialog before delete」。

- [ ] **步骤 1：编写失败测试**

```tsx
// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import type { RuntimeKnobs } from '../api'
import { GateContext } from '../gateContext'
import { RUNTIME } from '../strings'
import { RuntimeSettings } from './RuntimeSettings'

const baseKnobs: RuntimeKnobs = {
  max_messages: 40,
  max_steps: 16,
  tool_timeout_seconds: 60,
  compaction_enabled: true,
  compact_threshold: 0.8,
  compact_reserve_tokens: 8000,
  compact_keep_recent: 8,
  compact_summary_timeout_seconds: 60,
}

const overridden = Object.fromEntries(
  Object.keys(baseKnobs).map((k) => [k, false]),
) as api.RuntimeKnobsOverrides

async function renderRuntime() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  vi.spyOn(api, 'getRuntimeSettings').mockResolvedValue({
    effective: baseKnobs,
    overridden,
  })
  vi.spyOn(api, 'getCredentials').mockResolvedValue({
    source: 'override',
    operator_set: true,
    admin_set: true,
    operators: [],
  })
  await act(async () => {
    root.render(
      createElement(
        GateContext.Provider,
        { value: { role: 'admin', gateEnabled: true, operatorId: 'admin' } },
        createElement(RuntimeSettings),
      ),
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

describe('RuntimeSettings reset credentials ConfirmDialog', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('opens ConfirmDialog instead of window.confirm; confirms then patches reset', async () => {
    const patch = vi.spyOn(api, 'patchCredentials').mockResolvedValue({
      source: 'config',
      operator_set: true,
      admin_set: true,
      operators: [],
    })
    const confirmSpy = vi.spyOn(window, 'confirm')
    const { host, root } = await renderRuntime()

    const btn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('重置为基线口令'),
    )
    expect(btn).toBeTruthy()
    await act(async () => { btn!.click() })

    expect(confirmSpy).not.toHaveBeenCalled()
    expect(host.textContent).toContain(RUNTIME.confirmResetTitle)
    expect(patch).not.toHaveBeenCalled()

    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(patch).toHaveBeenCalledWith({ reset: true })
    root.unmount()
    host.remove()
  })
})
```

与 `ModelSettings.test.tsx` 相同：`GateContext.Provider` + `createElement`。knobs mock 对齐 `runtimeSettingsHelpers.test.ts` 的 `baseKnobs`。

- [ ] **步骤 2：运行测试确认失败**

```powershell
cd web/chat
npm test -- src/pages/RuntimeSettings.test.tsx
```

预期：FAIL（仍调用 `window.confirm` 或无 ConfirmDialog）。

- [ ] **步骤 3：实现**

在 `strings.ts` 增加例如：

```ts
export const RUNTIME = {
  confirmResetTitle: '重置为基线口令？',
  confirmResetBody: '将清空全部热更新凭据，回落到 YAML/env 基线口令。引擎参数不受影响。',
  confirmResetOk: '重置',
} as const
```

在 `CredentialsSection`：

- `const [pendingReset, setPendingReset] = useState(false)`
- 按钮 `onClick={() => setPendingReset(true)}`（不再直接 `resetAll`）
- `resetAll` 去掉 `window.confirm`；成功/失败后 `setPendingReset(false)`
- 渲染：

```tsx
<ConfirmDialog
  open={pendingReset}
  danger
  title={RUNTIME.confirmResetTitle}
  body={RUNTIME.confirmResetBody}
  confirmText={RUNTIME.confirmResetOk}
  busy={busy}
  error={/* 可选：重置失败时的人话 */}
  onCancel={() => setPendingReset(false)}
  onConfirm={() => void resetAll()}
/>
```

从 `../components/ui`（或项目现有 barrel）引入 `ConfirmDialog`，与 ModelSettings 一致。

- [ ] **步骤 4：运行测试确认通过**

```powershell
cd web/chat
npm test -- src/pages/RuntimeSettings.test.tsx
```

预期：PASS。

- [ ] **步骤 5：Commit**

```powershell
git add web/chat/src/pages/RuntimeSettings.tsx web/chat/src/pages/RuntimeSettings.test.tsx web/chat/src/strings.ts
git commit -m "fix(web): 运行参数重置凭据改用 ConfirmDialog"
```

---

## 任务 4：走查与中高优修复

**文件：**
- 创建：`docs/superpowers/notes/2026-09-13-webui-p4-acceptance.md`（先建骨架，边走边勾）
- 修改：`docs/superpowers/notes/2026-09-10-webui-p1-chat-visual-acceptance.md`（待复测 → 指向 P4 结果）
- 按需：`web/chat/src/styles/*`、相关页面/组件 + 对应 `*.test.tsx`

- [ ] **步骤 1：写 acceptance note 骨架**

复制规格 §4 矩阵表到 `docs/superpowers/notes/2026-09-13-webui-p4-acceptance.md`，增加「缺陷与修复」空表（ID / 现象 / 修复提交或「已知债」）。

- [ ] **步骤 2：启动本地实例并走查**

按规格 §4.1–§4.4 执行（明/暗 × 三宽度；聊天 C-*；设置代表页；运营角色抽测；a11y Tab/焦点）。

可用：`baize start` 或仓库惯用 demo 命令，独立数据目录，管理员 + 运营各登录一次。

- [ ] **步骤 3：修复每个中高优缺陷**

对每一项：

1. 能单测 → 先写失败测试再改（TDD）。
2. 仅视觉/布局难单测 → 修样式令牌化，在 acceptance note 标「仅人工」并写复测结果。
3. 纯文案偏好 → 记 note，**不**阻塞关门。
4. 大范围 IA → 记 backlog，**不**扩本计划。

每修完一组相关缺陷可单独 commit，例如：`fix(web): 修复暗色聊天区浅色穿帮`。

- [ ] **步骤 4：关闭 P1 验收表遗留**

在 `2026-09-10-webui-p1-chat-visual-acceptance.md` 将待复测项改为 ✅/已知债，并链到 P4 note。

- [ ] **步骤 5：提交 note（若尚未随修复提交）**

```powershell
git add docs/superpowers/notes/2026-09-13-webui-p4-acceptance.md docs/superpowers/notes/2026-09-10-webui-p1-chat-visual-acceptance.md
git commit -m "docs(webui): 记录 P4 走查验收结果"
```

---

## 任务 5：README 导航术语对齐

**文件：**
- 修改：`README.zh-CN.md`
- 修改：`README.md`

对照 `web/chat/src/settingsNav.ts` 人话标签：

| 旧用户路径说法 | 新人话 |
|----------------|--------|
| Settings → Tools / 设置 → Tools | 助手功能 |
| Settings → OpenAPI / 设置 → OpenAPI | 业务系统 |
| Settings → MCP | 外部工具服务 |
| Settings → MCP export / MCP 导出 | 对外提供能力 |
| Settings → Plugins / 插件 | 插件（可保持） |
| Settings → Webhook | 消息回调 |
| Settings → Inbox | 外部来信 |
| Settings → Channels / Weixin | 微信 |
| Settings → Models / 模型 | 模型 |
| Settings → Skills / 技能 | 技能 |
| Settings → Identities / 账号 | 账号 |

- [ ] **步骤 1：改中文 README 用户路径**

只改「打开 `/ui` → **设置 → …**」类导航说明。  
**不要**改：`PUT /v0/connectors`、JSON 字段名、`type: openapi`、curl、表头里的协议名「OpenAPI」。

例：`设置 → Tools` → `设置 → 助手功能`；`设置 → OpenAPI` → `设置 → 业务系统`；`设置 → Inbox` → `设置 → 外部来信`；`设置 → MCP 导出` → `设置 → 对外提供能力`。

执行回调说明若仍写「在 Tools 组头编辑」，改为「在业务系统 / 插件连接的高级设置中编辑」（与 IA 收口一致）。导出策略若仍写「在 Tools 调整」，改为「在对外提供能力页按工具配置」。

- [ ] **步骤 2：改英文 README 对等路径**

例：`Settings → Tools` → `Settings → Assistant capabilities`（或与 UI 英文化策略一致；若 UI 仅中文，英文 README 可用括号注明中文导航名：`Settings → 助手功能 (Assistant tools)`）。**优先与 `settingsNav` 中文标签一致并加简短英文解释**，避免发明第二套英文 IA。

- [ ] **步骤 3：自检**

```powershell
Select-String -Path README.md,README.zh-CN.md -Pattern 'Settings → Tools|设置 → Tools|Settings → OpenAPI|设置 → OpenAPI|Settings → Inbox|设置 → Inbox|Settings → MCP export|设置 → MCP 导出'
```

预期：用户导航语境无命中（API/协议叙述中的 OpenAPI 单词可保留）。

- [ ] **步骤 4：Commit**

```powershell
git add README.md README.zh-CN.md
git commit -m "docs: 对齐 README 设置导航人话用语"
```

---

## 任务 6：全量验证 + 重建 dist + DoD

**文件：**
- 修改：`internal/ui/dist/**`
- 修改：`docs/superpowers/notes/2026-09-13-webui-p4-acceptance.md`（勾完 DoD）
- 修改：`docs/superpowers/specs/2026-09-13-webui-refresh-p4-wrapup-design.md`（状态改为已交付）

- [ ] **步骤 1：全量前端检查**

```powershell
cd web/chat
npm test
npx tsc --noEmit
npm run build
```

预期：全部通过；`internal/ui/dist` 已更新。

- [ ] **步骤 2：确认无 style.css、无 window.confirm**

```powershell
Test-Path web/chat/src/style.css   # 应为 False
Select-String -Path web/chat/src -Pattern 'window\.confirm' -Recurse
```

预期：无 `style.css`；无 `window.confirm`（或仅测试里的 spy）。

- [ ] **步骤 3：勾选规格 DoD，更新规格状态为已交付**

- [ ] **步骤 4：Commit**

```powershell
git add internal/ui/dist web/chat docs/superpowers/notes/2026-09-13-webui-p4-acceptance.md docs/superpowers/specs/2026-09-13-webui-refresh-p4-wrapup-design.md
git commit -m "chore(ui): 重建嵌入产物并关闭 WebUI P4"
```

- [ ] **步骤 5：推送 real（若用户未禁止）**

```powershell
git push real main
```

不要推 `public`，除非用户明确说「推开源仓」。

---

## 规格覆盖自检

| 规格章节 | 任务 |
|----------|------|
| §1 成功标准 / 目标 | 任务 4–6 |
| §3 范围 CSS/门禁/Confirm/README/dist | 任务 1–3、5–6 |
| §4 走查矩阵 / a11y / 程序化门禁 | 任务 2、4 |
| §5 工程与测试 | 任务 1–3、6 |
| §5.4 README | 任务 5 |
| §7 DoD | 任务 6 |
| 非目标（E2E/F/消息组全改） | 无对应任务（正确） |

占位符扫描：计划内无 TBD/「适当处理」；Runtime mock 形状要求「以真实类型为准」已写明调整规则。

---

## 执行交接

计划已保存。两种执行方式：

**1. 子代理驱动（推荐）** — 每任务新子代理 + 任务间审查  

**2. 内联执行** — 本会话用 executing-plans，批量推进并设检查点  

选哪种方式？
