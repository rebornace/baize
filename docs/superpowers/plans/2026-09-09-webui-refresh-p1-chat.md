# WebUI 改版 P1：聊天重构 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 把聊天界面"默认说人话、技术按需展开"——结构化错误与人话提示、Toast/确认框/溢出菜单、工具/HITL/workflow 友好化（含历史回看）、模型芯片与选择持久化、克制欢迎区。

**架构：** 纯前端分层推进（`web/chat`，React 19 + Vite 6 + vitest），零后端契约改动。自底向上：文案/错误层 → 浮层组件 → 聊天卡片 → 模型芯片与接线 → 重建嵌入产物。技术名/JSON 一律保留在"详情"中。

**技术栈：** React 19、react-router-dom 7、Vite 6、TypeScript、vitest（静态渲染用 node 环境，交互用 `// @vitest-environment jsdom` + `react-dom/client` + `act`，不引 testing-library）、`lucide-react`（P0 已装）、P0 设计令牌。

**规格：** `docs/superpowers/specs/2026-09-09-webui-refresh-p1-chat.md`

**工作目录：** 所有前端命令在 `web/chat` 下运行；构建产物落到仓库 `internal/ui/dist`（vite 已配置）。

**通用验收（每个任务都要满足）：**
- `npx tsc --noEmit` 零错误；`npx vitest run` 全绿。
- 新样式只许引用 `styles/tokens.css` 中的令牌，禁止硬编码色值。
- 交互组件测试文件顶部加 `// @vitest-environment jsdom`，参照 `src/components/ui/ThemeToggle.test.tsx` 的 `createRoot + act` 范式。
- 提交信息用中文 conventional commits（`feat(web):` / `fix(web):`）。

---

## 文件结构（先锁定边界与职责）

新增：
- `web/chat/src/strings.ts` — 集中展示文案：错误码→人话、档位/能力/动作/HITL/欢迎区/高级项文案。纯函数，无 React。
- `web/chat/src/friendlyTool.ts` — `friendlyToolName` / `toolPhrase` 纯函数（工具技术名 + 目录 → 友好名/动作短语）。
- `web/chat/src/historyBlocks.ts` — `foldToolBlocks(runId, events)` 纯函数：从持久化事件只折叠工具/流程块，剔除 `llm.message`。
- `web/chat/src/components/ui/Toast.tsx` + `Toast.test.tsx` — `useToast`、`<ToastRegion/>`。
- `web/chat/src/components/ui/Modal.tsx` + `Modal.test.tsx` — 通用模态底座。
- `web/chat/src/components/ui/ConfirmDialog.tsx` + `ConfirmDialog.test.tsx` — 基于 Modal 的确认框。
- `web/chat/src/components/ui/DropdownMenu.tsx` + `DropdownMenu.test.tsx` — `useDropdown` + 菜单渲染。
- `web/chat/src/components/ModelChip.tsx` + `ModelChip.test.tsx` — 输入框内模型芯片（替代独立行下拉）。
- `web/chat/src/modelChoice.ts` + `modelChoice.test.ts` — 模型选择的 localStorage 读写与失效校验（纯函数）。

修改：
- `web/chat/src/api.ts` — 主 `parseJSON` 改为抛 `ApiError`（带 code，message 保留 `HTTP <status>:` 前缀以兼容现有门禁判断）。
- `web/chat/src/modelSelect.ts` — 档位/能力/Auto 展示文案更名（内部 id 不变）。
- `web/chat/src/components/ToolCard.tsx` — 友好步骤行 + 详情展开 + HITL"需要你确认"（仅拒绝可留言）。
- `web/chat/src/components/WorkflowCard.tsx` — "第 n/共 m 步" + 序号/中文名。
- `web/chat/src/components/Composer.tsx` — 接收模型芯片槽位（可选 `toolbar`），不改发送/附件主逻辑。
- `web/chat/src/pages/ChatPage.tsx` — 挂 ToastRegion/ConfirmDialog；拉工具目录；historyBlocks 接线；消息溢出菜单；欢迎区与高级项文案；模型选择持久化；删除对话走 ConfirmDialog；图片能力拦截走 Modal。
- `web/chat/src/styles/components.css` — 追加 toast/modal/dropdown 样式。
- `web/chat/src/style.css` — 工具卡/欢迎区/模型芯片/消息菜单的聊天级样式令牌化。
- `web/chat/src/components/ui/index.ts` — 导出新组件。

---

## 任务 1：文案/错误基础层（`strings.ts`）+ 主 `parseJSON` 抛 `ApiError` + 档位更名

**文件：**
- 创建：`web/chat/src/strings.ts`
- 测试：`web/chat/src/strings.test.ts`
- 修改：`web/chat/src/api.ts`（主 `parseJSON`，约 59-69 行；`ApiError` 类已存在于 882 行，勿重建）
- 修改：`web/chat/src/modelSelect.ts`（档位/能力/Auto 文案）
- 修改（仅文案，外溢）：`web/chat/src/pages/ModelSettings.tsx`（档位选项、徽标、说明）
- 测试更新：`web/chat/src/pages/ChatPageModelSelect.test.tsx`、`web/chat/src/pages/ModelSettings.test.tsx`

- [ ] **步骤 1.1：编写 `strings.ts` 的失败测试**

创建 `web/chat/src/strings.test.ts`：

```ts
import { describe, expect, it } from 'vitest'
import { ApiError } from './api'
import {
  AUTO_LABEL,
  ACTIONS,
  friendlyError,
  HITL,
  tierLabel,
  VISION_LABEL,
  WELCOME,
} from './strings'

describe('tierLabel', () => {
  it('maps internal tier ids to mainstream labels', () => {
    expect(tierLabel('light')).toBe('快速')
    expect(tierLabel('standard')).toBe('标准')
    expect(tierLabel('power')).toBe('深度思考')
  })
  it('falls back to 标准 for unknown/empty tier', () => {
    expect(tierLabel(undefined)).toBe('标准')
    expect(tierLabel('nope')).toBe('标准')
  })
})

describe('labels', () => {
  it('exposes friendly auto/vision/action/hitl/welcome strings', () => {
    expect(AUTO_LABEL).toBe('智能选择')
    expect(VISION_LABEL).toBe('能看图')
    expect(ACTIONS.copy).toBe('复制')
    expect(ACTIONS.regenerate).toBe('重新回答')
    expect(ACTIONS.editAndReanswer).toBe('编辑后重新回答')
    expect(ACTIONS.forkAsNew).toBe('复制成新对话')
    expect(ACTIONS.rollbackHere).toBe('回到这里')
    expect(HITL.title).toBe('需要你确认')
    expect(HITL.approve).toBe('同意')
    expect(HITL.reject).toBe('拒绝')
    expect(WELCOME.title).toBe('有什么可以帮你？')
  })
})

describe('friendlyError', () => {
  it('maps known ApiError codes to Chinese titles', () => {
    expect(friendlyError(new ApiError(409, 'conversation_busy', 'busy')).title).toContain('稍候')
    expect(friendlyError(new ApiError(400, 'no_model_configured', 'x')).title).toContain('AI 模型')
    expect(friendlyError(new ApiError(400, 'vision_unsupported', 'x')).title).toContain('图片')
    expect(friendlyError(new ApiError(401, 'unauthorized', 'x')).title).toContain('权限')
    expect(friendlyError(new ApiError(500, 'internal_error', 'x')).title).toContain('稍后')
    expect(friendlyError(new ApiError(404, 'not_found', 'x')).title).toContain('不存在')
  })
  it('maps invalid_signature explicitly', () => {
    expect(friendlyError(new ApiError(401, 'invalid_signature', 'bad sig')).title).toContain('刷新')
  })
  it('falls back to a generic title with detail for unknown codes', () => {
    const r = friendlyError(new ApiError(418, 'weird_teapot', 'boom'))
    expect(r.title).toBeTruthy()
    expect(r.detail).toContain('weird_teapot')
    expect(r.detail).toContain('boom')
  })
  it('treats fetch/network failures as network errors', () => {
    expect(friendlyError(new TypeError('Failed to fetch')).title).toContain('网络')
    expect(friendlyError(new Error('NetworkError when attempting to fetch resource')).title).toContain('网络')
  })
  it('falls back safely for non-Error values', () => {
    expect(friendlyError('nope').title).toBeTruthy()
  })
})
```

- [ ] **步骤 1.2：运行测试确认失败**

运行：`npx vitest run src/strings.test.ts`
预期：FAIL（`strings.ts` 不存在 / 导出缺失）。

- [ ] **步骤 1.3：实现 `strings.ts`**

创建 `web/chat/src/strings.ts`：

```ts
import { ApiError } from './api'

// ---- 模型档位 / 能力（内部 id 不变，仅展示更名）----
export type TierId = 'light' | 'standard' | 'power'

export const TIER_LABELS: Record<TierId, string> = {
  light: '快速',
  standard: '标准',
  power: '深度思考',
}

/** 内部档位 id -> 对外叫法；未知值按「标准」。 */
export function tierLabel(tier?: string): string {
  if (tier === 'light' || tier === 'standard' || tier === 'power') {
    return TIER_LABELS[tier]
  }
  return TIER_LABELS.standard
}

export const AUTO_LABEL = '智能选择'
export const VISION_LABEL = '能看图'

// ---- 聊天动作 ----
export const ACTIONS = {
  copy: '复制',
  regenerate: '重新回答',
  editAndReanswer: '编辑后重新回答',
  forkAsNew: '复制成新对话',
  rollbackHere: '回到这里',
  more: '更多操作',
} as const

// ---- HITL ----
export const HITL = {
  title: '需要你确认',
  approve: '同意',
  reject: '拒绝',
  commentPlaceholder: '留言（选填）',
  approved: '已同意',
  rejected: '已拒绝',
} as const

// ---- 欢迎区 ----
export const WELCOME = {
  title: '有什么可以帮你？',
  subtitle: '我可以查询数据、调用业务系统、处理文件与图片，直接说出你的需求即可。',
} as const

// ---- 高级项 ----
export const ADVANCED = {
  summary: '高级',
  tokenLabel: '临时访问凭证（选填）',
  tokenHint: '需要带身份访问时填写，仅本次会话使用。',
  webhookLabel: '本次结果回调地址（选填）',
} as const

export interface FriendlyError {
  title: string
  /** 技术细节，默认收起，供「详情」展开。 */
  detail?: string
}

const CODE_TITLE: Record<string, string> = {
  conversation_busy: '上一条还在处理中，请稍候再发。',
  no_model_configured: '还没有可用的 AI 模型，请到「设置 → AI 模型」添加一个。',
  vision_unsupported: '当前模型看不了图片，请改用「智能选择」或带「能看图」标记的模型。',
  invalid_signature: '连接校验未通过，请刷新页面后重试。',
  not_found: '内容不存在或已被删除。',
  internal_error: '服务暂时出了点问题，请稍后重试。',
}

function isNetworkish(e: unknown): boolean {
  const msg = e instanceof Error ? e.message : String(e ?? '')
  return /failed to fetch|networkerror|load failed|network request/i.test(msg)
}

/** 把任意抛出值翻译为人话标题；未知错误附带可展开的技术 detail。 */
export function friendlyError(e: unknown): FriendlyError {
  if (e instanceof ApiError) {
    if (e.status === 401 || e.status === 403) {
      return { title: '没有访问权限或登录已失效，请重新解锁后再试。' }
    }
    if (e.status >= 500 && !CODE_TITLE[e.code]) {
      return { title: CODE_TITLE.internal_error, detail: `${e.code}: ${e.message}` }
    }
    const title = CODE_TITLE[e.code]
    if (title) return { title }
    return { title: '操作未能完成，请稍后重试。', detail: `${e.code}: ${e.message}` }
  }
  if (isNetworkish(e)) {
    return { title: '网络连接失败，请检查服务是否正在运行。' }
  }
  const detail = e instanceof Error ? e.message : String(e ?? '')
  return { title: '出现了未知问题。', detail: detail || undefined }
}
```

- [ ] **步骤 1.4：运行测试确认通过**

运行：`npx vitest run src/strings.test.ts`
预期：PASS。

- [ ] **步骤 1.5：主 `parseJSON` 改为抛 `ApiError`（保留 `HTTP <status>:` 前缀）**

`web/chat/src/api.ts` 主 `parseJSON`（约 59 行）替换为：

```ts
async function parseJSON<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let code = 'unknown'
    let message = res.statusText
    try {
      const body = (await res.json()) as { error?: { code?: string; message?: string } }
      if (body.error?.code) code = body.error.code
      if (body.error?.message) message = body.error.message
    } catch {
      /* ignore */
    }
    // 保留 "HTTP <status>:" 前缀，兼容 GateRoot 的 startsWith('HTTP 401:') 判断。
    throw new ApiError(res.status, code, `HTTP ${res.status}: ${message}`)
  }
  return (await res.json()) as T
}
```

注：`ApiError` 类已存在（同文件约 882 行），不要重复定义；`parseConnectorJSON` 保持原样不动。

- [ ] **步骤 1.6：`modelSelect.ts` 档位/能力/Auto 文案更名（内部 id 不变）**

在 `modelSelect.ts` 顶部改为从 `strings` 引入，并更新函数：

```ts
import { AUTO_LABEL, tierLabel, VISION_LABEL } from './strings'
```

`modelOptions` 内：
- `const auto = { value: AUTO_MODEL_ID, label: AUTO_LABEL }`
- tags：`p.supports_vision ? VISION_LABEL : ''`
- `tierLabel` 现在来自 `./strings`（删除本文件内旧的 `tierLabel` 实现，改为 `export { tierLabel } from './strings'` 以保持 `ModelSettings` 既有 import 路径可用）。

`visionGate` 的两条 message 更新为：
- 无视觉模型：`当前没有可用的「能看图」模型，请到「设置 → AI 模型」添加，或移除图片后再发送。`
- 手选不支持：`这个模型看不了图片。请改用「智能选择」或带「能看图」标记的模型，或移除图片。`

- [ ] **步骤 1.7：同步 `ModelSettings.tsx` 档位文案（仅文案）**

- AutoTier 下拉选项（约 15-18 行）改为：
  - `light`：`快速（简单、省钱的日常任务）`
  - `standard`：`标准（适合多数任务）`
  - `power`：`深度思考（复杂推理、长任务）`
- 视觉复选框 label（约 218 行）：`能看图（支持图片附件）`
- 模型徽标 `视觉`（约 292 行）改为 `{p.supports_vision && <span className="settings-badge">{VISION_LABEL}</span>}`，并 `import { VISION_LABEL } from '../strings'`。
- 说明段落（约 258、468-470 行）中的「轻量/标准/强力」改为「快速/标准/深度思考」，「支持视觉」改为「能看图」，「智能路由（Auto）」改为「智能选择」。不改任何字段名/控件。

- [ ] **步骤 1.8：更新受影响的现有测试断言**

`ChatPageModelSelect.test.tsx`：
- 第 35、54 行 `label: '智能路由（Auto）'` → `label: '智能选择'`
- 第 42 行 → `{ value: 'mp_2', label: '轻量（gpt-4o-mini） · 快速' }`
- 第 50 行 → `expect(opts[1]).toEqual({ value: 'mp_v', label: '视觉（gpt-4o） · 标准·能看图' })`
- 第 92 行 `expect(r.message).toContain('视觉模型')` → `toContain('能看图')`
- 第 102 行 `toContain('智能路由')` → `toContain('智能选择')`
- 第 132 行 `toContain('智能路由（Auto）')` → `toContain('智能选择')`
- 第 135 行 → `toContain('轻量（gpt-4o-mini） · 快速')`

`ModelSettings.test.tsx`：
- 该文件 `shows tier + vision badges` 用例：把对档位的断言改为新文案。该用例 profiles 里有 `name: '轻量模型'` 与 `auto_tier:'light'`，名称本身仍含“轻量”二字，因此不要再用 `toContain('轻量')` 断言档位；改为断言 light 档位选项文案 `快速（简单` 出现在页面，或断言 `标准模型` 对应徽标 `标准`。视觉断言 `html.match(/视觉/g)` 改为 `html.match(/能看图/g)`，且 `expect(...).toBeGreaterThanOrEqual(1)` 保持不变。

- [ ] **步骤 1.9：全量校验并提交**

运行：`npx tsc --noEmit`；`npx vitest run`
预期：零错误、全绿。

```bash
git add web/chat/src/strings.ts web/chat/src/strings.test.ts web/chat/src/api.ts web/chat/src/modelSelect.ts web/chat/src/pages/ModelSettings.tsx web/chat/src/pages/ChatPageModelSelect.test.tsx web/chat/src/pages/ModelSettings.test.tsx
git commit -m "feat(web): P1 文案/错误基础层——ApiError 统一、错误码人话映射、档位更名快速/标准/深度思考"
```

## 任务 2：Toast 反馈层（`useToast` + `ToastRegion`）

**文件：**
- 创建：`web/chat/src/components/ui/Toast.tsx`
- 测试：`web/chat/src/components/ui/Toast.test.tsx`
- 修改：`web/chat/src/styles/components.css`（追加 toast 样式）
- 修改：`web/chat/src/components/ui/index.ts`（导出）

- [ ] **步骤 2.1：编写失败测试（jsdom 交互）**

创建 `web/chat/src/components/ui/Toast.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastRegion, useToast } from './Toast'

let host: HTMLDivElement
function renderRegion(): { push: ReturnType<typeof useToast>['push'] } {
  let api!: ReturnType<typeof useToast>
  function Harness() {
    api = useToast()
    return <ToastRegion toasts={api.toasts} onDismiss={api.dismiss} />
  }
  act(() => {
    createRoot(host).render(<Harness />)
  })
  return { push: (...a: Parameters<typeof api.push>) => act(() => { api.push(...a) }) }
}

beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  vi.useFakeTimers()
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => {
  host.remove()
  vi.useRealTimers()
})

describe('ToastRegion', () => {
  it('renders a toast with polite live region and auto-dismisses', () => {
    const { push } = renderRegion()
    push({ tone: 'success', title: '已复制' })
    expect(host.textContent).toContain('已复制')
    const region = host.querySelector('[aria-live="polite"]')
    expect(region).not.toBeNull()
    act(() => { vi.advanceTimersByTime(4100) })
    expect(host.textContent).not.toContain('已复制')
  })

  it('keeps error toasts longer (6s) and allows manual close', () => {
    const { push } = renderRegion()
    push({ tone: 'error', title: '出错了', detail: 'boom' })
    act(() => { vi.advanceTimersByTime(4100) })
    expect(host.textContent).toContain('出错了')
    const close = host.querySelector('[data-testid="toast-close"]') as HTMLButtonElement
    act(() => { close.click() })
    expect(host.textContent).not.toContain('出错了')
  })

  it('caps the stack at 3 toasts', () => {
    const { push } = renderRegion()
    push({ tone: 'info', title: '一' })
    push({ tone: 'info', title: '二' })
    push({ tone: 'info', title: '三' })
    push({ tone: 'info', title: '四' })
    expect(host.textContent).not.toContain('一')
    expect(host.textContent).toContain('四')
    expect(host.querySelectorAll('[data-testid="toast"]').length).toBe(3)
  })
})
```

- [ ] **步骤 2.2：运行确认失败**

运行：`npx vitest run src/components/ui/Toast.test.tsx`
预期：FAIL（模块不存在）。

- [ ] **步骤 2.3：实现 `Toast.tsx`**

```tsx
import { useCallback, useRef, useState } from 'react'

export type ToastTone = 'success' | 'error' | 'info'

export interface ToastInput {
  tone: ToastTone
  title: string
  detail?: string
}

export interface ToastItem extends ToastInput {
  id: number
}

const DURATION_MS: Record<ToastTone, number> = {
  success: 4000,
  info: 4000,
  error: 6000,
}
const MAX_TOASTS = 3

export interface ToastApi {
  toasts: ToastItem[]
  push: (t: ToastInput) => void
  dismiss: (id: number) => void
}

export function useToast(): ToastApi {
  const [toasts, setToasts] = useState<ToastItem[]>([])
  const nextId = useRef(1)

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const push = useCallback(
    (t: ToastInput) => {
      const id = nextId.current++
      setToasts((prev) => [...prev, { ...t, id }].slice(-MAX_TOASTS))
      window.setTimeout(() => dismiss(id), DURATION_MS[t.tone])
    },
    [dismiss],
  )

  return { toasts, push, dismiss }
}

export function ToastRegion({
  toasts,
  onDismiss,
}: {
  toasts: ToastItem[]
  onDismiss: (id: number) => void
}) {
  return (
    <div className="toast-region" aria-live="polite" aria-atomic="false">
      {toasts.map((t) => (
        <div
          key={t.id}
          data-testid="toast"
          className={`toast toast-${t.tone}`}
          role="status"
        >
          <div className="toast-body">
            <p className="toast-title">{t.title}</p>
            {t.detail && <p className="toast-detail">{t.detail}</p>}
          </div>
          <button
            type="button"
            className="toast-close"
            data-testid="toast-close"
            aria-label="关闭提示"
            onClick={() => onDismiss(t.id)}
          >
            ×
          </button>
        </div>
      ))}
    </div>
  )
}
```

- [ ] **步骤 2.4：追加样式到 `components.css`（只用令牌）**

```css
/* ---- Toast ---- */
.toast-region {
  position: fixed;
  top: var(--space-4);
  right: var(--space-4);
  z-index: 1000;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  max-width: min(360px, calc(100vw - 2 * var(--space-4)));
}
.toast {
  display: flex;
  gap: var(--space-2);
  align-items: flex-start;
  background: var(--surface);
  border: 1px solid var(--border);
  border-left-width: 4px;
  border-radius: var(--radius);
  box-shadow: var(--shadow-2);
  padding: var(--space-3) var(--space-4);
}
.toast-success { border-left-color: var(--success); }
.toast-error { border-left-color: var(--danger); }
.toast-info { border-left-color: var(--info); }
.toast-title { margin: 0; font-weight: 600; color: var(--text); }
.toast-detail { margin: 4px 0 0; font-size: 13px; color: var(--text-muted); word-break: break-word; }
.toast-close {
  margin-left: auto;
  border: 0;
  background: transparent;
  color: var(--text-muted);
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
}
.toast-close:focus-visible { outline: 2px solid var(--info); outline-offset: 2px; }
```

- [ ] **步骤 2.5：从 barrel 导出并校验**

在 `components/ui/index.ts` 追加：

```ts
export { ToastRegion, useToast, type ToastInput, type ToastTone, type ToastApi } from './Toast'
```

运行：`npx vitest run src/components/ui/Toast.test.tsx`；`npx tsc --noEmit`
预期：PASS、零错误。

- [ ] **步骤 2.6：提交**

```bash
git add web/chat/src/components/ui/Toast.tsx web/chat/src/components/ui/Toast.test.tsx web/chat/src/styles/components.css web/chat/src/components/ui/index.ts
git commit -m "feat(web): P1 Toast 反馈层（自动消失/手动关闭/最多3条/aria-live）"
```

## 任务 3：Modal 底座 + ConfirmDialog（受控确认框）

**文件：**
- 创建：`web/chat/src/components/ui/Modal.tsx` + `Modal.test.tsx`
- 创建：`web/chat/src/components/ui/ConfirmDialog.tsx` + `ConfirmDialog.test.tsx`
- 修改：`web/chat/src/styles/components.css`
- 修改：`web/chat/src/components/ui/index.ts`

- [ ] **步骤 3.1：编写 Modal 失败测试**

`Modal.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Modal } from './Modal'

let host: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div'); document.body.appendChild(host)
})
afterEach(() => { host.remove() })

function render(el: ReactNode) {
  act(() => { createRoot(host).render(el) })
}

describe('Modal', () => {
  it('renders nothing when closed, renders dialog when open with role/title', () => {
    render(<Modal open={false} title="T"><p>body</p></Modal>)
    expect(host.textContent).not.toContain('body')
    render(<Modal open title="T"><p>body</p></Modal>)
    const dlg = host.querySelector('[role="dialog"]')
    expect(dlg).not.toBeNull()
    expect(host.textContent).toContain('T')
    expect(host.textContent).toContain('body')
  })
  it('closes on Escape and overlay click, not on dialog content click', () => {
    const onClose = vi.fn()
    render(<Modal open title="T" onClose={onClose}><span data-testid="in">x</span></Modal>)
    act(() => {
      host.querySelector('[role="dialog"]')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    expect(onClose).toHaveBeenCalledOnce()
    act(() => { (host.querySelector('[data-testid="modal-overlay"]') as HTMLElement).click() })
    expect(onClose).toHaveBeenCalledTimes(2)
    onClose.mockClear()
    act(() => { (host.querySelector('[data-testid="modal-panel"]') as HTMLElement).click() })
    expect(onClose).not.toHaveBeenCalled()
  })
})
```

- [ ] **步骤 3.2：实现 `Modal.tsx`**

```tsx
import { useEffect, useRef, type ReactNode } from 'react'

export interface ModalProps {
  open: boolean
  title: string
  onClose?: () => void
  children: ReactNode
  footer?: ReactNode
}

export function Modal({ open, title, onClose, children, footer }: ModalProps) {
  const panelRef = useRef<HTMLDivElement>(null)
  const previousActive = useRef<Element | null>(null)

  useEffect(() => {
    if (!open) return
    previousActive.current = document.activeElement
    const close = onClose
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close?.()
    }
    document.addEventListener('keydown', onKey)
    const focusables = panelRef.current?.querySelectorAll<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    )
    ;(focusables?.[0] ?? panelRef.current)?.focus()
    return () => {
      document.removeEventListener('keydown', onKey)
      ;(previousActive.current as HTMLElement | null)?.focus?.()
    }
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      className="modal-overlay"
      data-testid="modal-overlay"
      onClick={(e) => { if (e.target === e.currentTarget) onClose?.() }}
    >
      <div
        ref={panelRef}
        className="modal-panel"
        data-testid="modal-panel"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
      >
        <h2 className="modal-title">{title}</h2>
        <div className="modal-content">{children}</div>
        {footer && <div className="modal-footer">{footer}</div>}
      </div>
    </div>
  )
}
```

- [ ] **步骤 3.3：追加 Modal 样式到 `components.css`**

```css
/* ---- Modal ---- */
.modal-overlay {
  position: fixed; inset: 0; z-index: 1100;
  background: rgba(15, 23, 42, 0.45);
  display: flex; align-items: center; justify-content: center;
  padding: var(--space-4);
}
.modal-panel {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-3);
  width: min(440px, 100%);
  padding: var(--space-6);
  outline: none;
}
.modal-title { margin: 0 0 var(--space-2); font-size: 17px; color: var(--text); }
.modal-content { color: var(--text); font-size: 14px; line-height: 1.6; }
.modal-footer { display: flex; justify-content: flex-end; gap: var(--space-2); margin-top: var(--space-4); }
```

- [ ] **步骤 3.4：运行 Modal 测试通过**

运行：`npx vitest run src/components/ui/Modal.test.tsx`，预期 PASS。

- [ ] **步骤 3.5：编写 ConfirmDialog 失败测试**

`ConfirmDialog.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ConfirmDialog } from './ConfirmDialog'

let host: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div'); document.body.appendChild(host)
})
afterEach(() => host.remove())
function render(el: ReactNode) { act(() => { createRoot(host).render(el) }) }

describe('ConfirmDialog', () => {
  it('shows consequence text and calls onConfirm/onCancel', () => {
    const onConfirm = vi.fn(); const onCancel = vi.fn()
    render(<ConfirmDialog open title="删除对话？" body="将永久删除，不可恢复。" confirmText="删除" danger onConfirm={onConfirm} onCancel={onCancel} />)
    expect(host.textContent).toContain('不可恢复')
    act(() => { (host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click() })
    expect(onConfirm).toHaveBeenCalledOnce()
    act(() => { (host.querySelector('[data-testid="confirm-cancel"]') as HTMLButtonElement).click() })
    expect(onCancel).toHaveBeenCalledOnce()
  })
  it('initial focus lands on the cancel button for dangerous confirms', () => {
    render(<ConfirmDialog open title="x" body="y" danger confirmText="删除" onConfirm={() => {}} onCancel={() => {}} />)
    const cancel = host.querySelector('[data-testid="confirm-cancel"]') as HTMLButtonElement
    expect(document.activeElement).toBe(cancel)
  })
})
```

- [ ] **步骤 3.6：实现 `ConfirmDialog.tsx`**

```tsx
import { Button } from './Button'
import { Modal } from './Modal'

export interface ConfirmDialogProps {
  open: boolean
  title: string
  body: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
  busy?: boolean
  onConfirm: () => void
  onCancel: () => void
}

export function ConfirmDialog({
  open, title, body, confirmText = '确认', cancelText = '取消',
  danger, busy, onConfirm, onCancel,
}: ConfirmDialogProps) {
  return (
    <Modal
      open={open}
      title={title}
      onClose={onCancel}
      footer={
        <>
          <Button data-testid="confirm-cancel" variant="secondary" onClick={onCancel} disabled={busy}>
            {cancelText}
          </Button>
          <Button
            data-testid="confirm-ok"
            variant={danger ? 'danger' : 'primary'}
            onClick={onConfirm}
            disabled={busy}
          >
            {confirmText}
          </Button>
        </>
      }
    >
      <p className="confirm-body">{body}</p>
    </Modal>
  )
}
```

注：`Button` 透传 `data-testid`（已 `...rest`）。危险操作“初始焦点在取消”由 Modal 的焦点顺序保证——cancel 按钮在 footer 中排第一，即成为第一个可聚焦元素。

- [ ] **步骤 3.7：导出并校验、提交**

`index.ts` 追加：

```ts
export { Modal, type ModalProps } from './Modal'
export { ConfirmDialog, type ConfirmDialogProps } from './ConfirmDialog'
```

运行：`npx vitest run src/components/ui/Modal.test.tsx src/components/ui/ConfirmDialog.test.tsx`；`npx tsc --noEmit`。

```bash
git add web/chat/src/components/ui/Modal.tsx web/chat/src/components/ui/Modal.test.tsx web/chat/src/components/ui/ConfirmDialog.tsx web/chat/src/components/ui/ConfirmDialog.test.tsx web/chat/src/styles/components.css web/chat/src/components/ui/index.ts
git commit -m "feat(web): P1 Modal 底座与 ConfirmDialog（Esc/遮罩关闭、危险操作焦点落取消）"
```

## 任务 4：DropdownMenu（`⋯` 溢出菜单，键盘/点外关闭）

**文件：**
- 创建：`web/chat/src/components/ui/DropdownMenu.tsx` + `DropdownMenu.test.tsx`
- 修改：`web/chat/src/styles/components.css`、`components/ui/index.ts`

- [ ] **步骤 4.1：编写失败测试**

`DropdownMenu.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DropdownMenu, type MenuItem } from './DropdownMenu'

let host: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div'); document.body.appendChild(host)
})
afterEach(() => host.remove())

const items: MenuItem[] = [
  { id: 'copy', label: '复制', onSelect: vi.fn() },
  { id: 'fork', label: '复制成新对话', onSelect: vi.fn() },
]
function render() {
  act(() => {
    createRoot(host).render(
      <DropdownMenu triggerLabel="更多操作" items={items} />,
    )
  })
}

describe('DropdownMenu', () => {
  it('toggles on trigger click and exposes aria-expanded/menuitem', () => {
    render()
    const trigger = host.querySelector('[data-testid="dropdown-trigger"]') as HTMLButtonElement
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    act(() => { trigger.click() })
    expect(trigger.getAttribute('aria-expanded')).toBe('true')
    expect(host.querySelectorAll('[role="menuitem"]').length).toBe(2)
  })
  it('selects an item, fires onSelect, and closes', () => {
    render()
    act(() => { (host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click() })
    act(() => { (host.querySelectorAll('[role="menuitem"]')[1] as HTMLElement).click() })
    expect(items[1].onSelect).toHaveBeenCalledOnce()
    expect((host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).getAttribute('aria-expanded')).toBe('false')
  })
  it('closes on Escape and on outside click', () => {
    render()
    act(() => { (host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click() })
    act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    expect(host.querySelector('[role="menu"]')).toBeNull()
    act(() => { (host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click() })
    act(() => { document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true })) })
    expect(host.querySelector('[role="menu"]')).toBeNull()
  })
  it('moves highlight with ArrowDown/Up and activates with Enter', () => {
    render()
    const trigger = host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement
    act(() => { trigger.click() })
    act(() => { trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true })) })
    act(() => {
      ;(host.querySelector('[role="menu"]') as HTMLElement).dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }),
      )
    })
    expect(items[0].onSelect).toHaveBeenCalledOnce()
  })
})
```

- [ ] **步骤 4.2：实现 `DropdownMenu.tsx`**

```tsx
import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react'

export interface MenuItem {
  id: string
  label: string
  icon?: ReactNode
  destructive?: boolean
  onSelect: () => void
}

export interface DropdownMenuProps {
  triggerLabel: string
  items: MenuItem[]
  triggerClassName?: string
  disabled?: boolean
  children?: ReactNode
}

export function DropdownMenu({ triggerLabel, items, triggerClassName, disabled = false, children }: DropdownMenuProps) {
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(0)
  const rootRef = useRef<HTMLDivElement>(null)
  const menuId = useId()

  const close = useCallback(() => setOpen(false), [])

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) close()
    }
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') close() }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, close])

  const choose = (item: MenuItem) => { item.onSelect(); close() }

  const onMenuKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setActive((a) => (a + 1) % items.length) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setActive((a) => (a - 1 + items.length) % items.length) }
    else if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); const it = items[active]; if (it) choose(it) }
  }

  const onTriggerKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown' && open) {
      e.preventDefault()
      setActive(0)
    }
  }

  return (
    <div className="dropdown" ref={rootRef}>
      <button
        type="button"
        className={`dropdown-trigger ${triggerClassName ?? ''}`.trim()}
        data-testid="dropdown-trigger"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        aria-label={triggerLabel}
        title={triggerLabel}
        disabled={disabled}
        onClick={() => { if (!disabled) { setOpen((v) => !v); setActive(0) } }}
        onKeyDown={onTriggerKey}
      >
        {children ?? '⋯'}
      </button>
      {open && (
        <div
          id={menuId}
          className="dropdown-menu"
          role="menu"
          aria-label={triggerLabel}
          onKeyDown={onMenuKey}
        >
          {items.map((item, i) => (
            <button
              key={item.id}
              type="button"
              role="menuitem"
              className={`dropdown-item${item.destructive ? ' danger' : ''}${i === active ? ' active' : ''}`}
              tabIndex={-1}
              onMouseEnter={() => setActive(i)}
              onClick={() => choose(item)}
              ref={(el) => { if (i === active && open) el?.focus() }}
            >
              {item.icon}
              <span>{item.label}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
```

注：焦点跟随高亮（ref 回调 focus active 项），因此 Enter 由菜单项/容器处理；测试里对 menu 容器派发 Enter 即命中 active 项。

- [ ] **步骤 4.3：追加样式**

```css
/* ---- DropdownMenu ---- */
.dropdown { position: relative; display: inline-flex; }
.dropdown-trigger {
  border: 0; background: transparent; color: var(--text-muted);
  min-width: 32px; min-height: 32px; border-radius: var(--radius-sm);
  cursor: pointer; font-size: 18px; line-height: 1; padding: 0 var(--space-1);
}
.dropdown-trigger:hover { background: var(--surface-2); color: var(--text); }
.dropdown-trigger:focus-visible { outline: 2px solid var(--info); outline-offset: 1px; }
.dropdown-trigger:disabled { opacity: .5; cursor: not-allowed; }
.dropdown-menu {
  position: absolute; right: 0; bottom: calc(100% + 6px);
  min-width: 180px; z-index: 1050;
  background: var(--surface); border: 1px solid var(--border);
  border-radius: var(--radius); box-shadow: var(--shadow-2);
  padding: var(--space-1); display: flex; flex-direction: column;
}
.dropdown-item {
  display: flex; align-items: center; gap: var(--space-2);
  border: 0; background: transparent; width: 100%;
  text-align: left; padding: 10px var(--space-3);
  border-radius: var(--radius-sm); color: var(--text);
  font-size: 14px; cursor: pointer; min-height: 40px;
}
.dropdown-item:hover, .dropdown-item.active { background: var(--surface-2); }
.dropdown-item.danger { color: var(--danger); }
.dropdown-item:focus-visible { outline: 2px solid var(--info); outline-offset: -2px; }
```

- [ ] **步骤 4.4：导出、校验、提交**

`index.ts` 追加：

```ts
export { DropdownMenu, type MenuItem, type DropdownMenuProps } from './DropdownMenu'
```

运行：`npx vitest run src/components/ui/DropdownMenu.test.tsx`；`npx tsc --noEmit`。

```bash
git add web/chat/src/components/ui/DropdownMenu.tsx web/chat/src/components/ui/DropdownMenu.test.tsx web/chat/src/styles/components.css web/chat/src/components/ui/index.ts
git commit -m "feat(web): P1 DropdownMenu 溢出菜单（点外/Esc 关闭、方向键、Enter、触摸目标≥40）"
```

## 任务 5：友好工具名与历史工具块折叠（纯函数）

**文件：**
- 创建：`web/chat/src/friendlyTool.ts` + `friendlyTool.test.ts`
- 创建：`web/chat/src/historyBlocks.ts` + `historyBlocks.test.ts`

- [ ] **步骤 5.1：编写 `friendlyTool` 失败测试**

`friendlyTool.test.ts`：

```ts
import { describe, expect, it } from 'vitest'
import { friendlyToolName, toolPhrase, type ToolCatalog } from './friendlyTool'

const catalog: ToolCatalog = [
  { name: 'query_order', title: '查询订单', description: '按单号查订单' },
  { name: 'no_title', title: '', description: '' },
]

describe('friendlyToolName', () => {
  it('prefers catalog title, falls back to technical name', () => {
    expect(friendlyToolName('query_order', catalog)).toBe('查询订单')
    expect(friendlyToolName('no_title', catalog)).toBe('no_title')
    expect(friendlyToolName('totally_new', catalog)).toBe('totally_new')
    expect(friendlyToolName('query_order', [])).toBe('query_order')
  })
})

describe('toolPhrase', () => {
  it('builds human phrases per status without fabricating verbs', () => {
    expect(toolPhrase('query_order', 'running', catalog)).toBe('正在处理：查询订单…')
    expect(toolPhrase('query_order', 'waiting_human', catalog)).toBe('待确认：查询订单')
    expect(toolPhrase('query_order', 'succeeded', catalog)).toBe('已完成：查询订单')
    expect(toolPhrase('query_order', 'failed', catalog)).toBe('处理失败：查询订单')
    expect(toolPhrase('query_order', 'approved', catalog)).toBe('已同意：查询订单')
    expect(toolPhrase('query_order', 'rejected', catalog)).toBe('已拒绝：查询订单')
  })
  it('uses technical name in phrase when no title', () => {
    expect(toolPhrase('totally_new', 'running', catalog)).toBe('正在处理：totally_new…')
  })
})
```

- [ ] **步骤 5.2：实现 `friendlyTool.ts`**

注意：`ToolCatalog` 不要依赖未导出的 `ToolInfo` 细节；直接定义为结构化类型，避免导入问题：

```ts
export interface ToolCatalogEntry {
  name: string
  title?: string
  description?: string
}
export type ToolCatalog = ToolCatalogEntry[]

type ToolStatus =
  | 'running'
  | 'waiting_human'
  | 'succeeded'
  | 'failed'
  | 'approved'
  | 'rejected'

const PHRASE: Record<ToolStatus, (label: string) => string> = {
  running: (l) => `正在处理：${l}…`,
  waiting_human: (l) => `待确认：${l}`,
  succeeded: (l) => `已完成：${l}`,
  failed: (l) => `处理失败：${l}`,
  approved: (l) => `已同意：${l}`,
  rejected: (l) => `已拒绝：${l}`,
}

/** 工具技术名 -> 友好名：有 title 用 title，否则回退技术名。 */
export function friendlyToolName(name: string, catalog: ToolCatalog): string {
  const hit = catalog.find((t) => t.name === name)
  const title = hit?.title?.trim()
  return title || name
}

/** 按状态生成动作短语；不对任意工具名硬拼动词。 */
export function toolPhrase(name: string, status: ToolStatus, catalog: ToolCatalog): string {
  return PHRASE[status](friendlyToolName(name, catalog))
}
```

（不要 `import type { ToolInfo } from './api'`；用上面的结构化 `ToolCatalogEntry`，与 `/v0/tools` 返回项的 name/title/description 结构兼容。）

- [ ] **步骤 5.3：编写 `historyBlocks` 失败测试**

先读 `web/chat/src/foldEvents.ts`，确认 `ChatBlock` 的判别字段与 `Event` 类型；若 `Event` 的 data 字段名与下面不同，以真实类型为准调整测试，但断言的行为不变（保留 tool/workflow、剔除文本）。

`historyBlocks.test.ts`：

```ts
import { describe, expect, it } from 'vitest'
import type { Event } from './api'
import { foldToolBlocks } from './historyBlocks'

function ev(type: string, data: Record<string, unknown>): Event {
  return { type, timestamp: '', data } as Event
}

describe('foldToolBlocks', () => {
  it('keeps tool/workflow blocks in event order and drops assistant text', () => {
    const events: Event[] = [
      ev('llm.tool_call', { name: 'a' }),
      ev('tool.result', { name: 'a', content: 'ok' }),
      ev('llm.message', { content: 'assistant words' }),
      ev('workflow.started', { skill: 's', steps: ['x'] }),
    ]
    const blocks = foldToolBlocks('run-1', events)
    expect(blocks.map((b) => b.kind)).toEqual(['tool', 'workflow'])
    const tool = blocks[0]
    expect(tool.kind === 'tool' && tool.name).toBe('a')
    expect(tool.kind === 'tool' && tool.status).toBe('succeeded')
    expect(blocks.every((b) => b.runId === 'run-1')).toBe(true)
  })
  it('returns [] when there are no tool/workflow events', () => {
    expect(foldToolBlocks('r', [ev('llm.message', { content: 'hi' })])).toEqual([])
  })
  it('folds HITL waiting into a tool block for read-only rendering', () => {
    const blocks = foldToolBlocks('r', [
      ev('llm.tool_call', { name: 'a' }),
      ev('hitl.waiting', { tool_name: 'a', arguments: { k: 1 } }),
    ])
    expect(blocks).toHaveLength(1)
    expect(blocks[0].kind === 'tool' && blocks[0].status).toBe('waiting_human')
  })
})
```

- [ ] **步骤 5.4：实现 `historyBlocks.ts`（复用 `foldEvents`，DRY）**

先读 `foldEvents.ts` 确认导出的类型名（`ChatBlock`）与函数签名（runId 形参名），再写：

```ts
import type { Event } from './api'
import { foldEvents, type ChatBlock } from './foldEvents'

export type ToolOrWorkflowBlock = Extract<ChatBlock, { kind: 'tool' | 'workflow' }>

/**
 * 从持久化 run 事件中只折叠工具/流程块（剔除 llm.message 等文本块），
 * 供历史回看在对应助手消息处展示，避免与已持久化的助手文本重复。
 */
export function foldToolBlocks(runId: string, events: Event[]): ToolOrWorkflowBlock[] {
  return foldEvents(runId, events).filter(
    (b): b is ToolOrWorkflowBlock => b.kind === 'tool' || b.kind === 'workflow',
  )
}
```

若真实 `foldEvents` 的第二个位置参数不叫 `events` 或返回类型不含这两个 kind，按真实签名调整（行为不变）。

- [ ] **步骤 5.5：运行并提交**

运行：`npx vitest run src/friendlyTool.test.ts src/historyBlocks.test.ts`；`npx tsc --noEmit`。

```bash
git add web/chat/src/friendlyTool.ts web/chat/src/friendlyTool.test.ts web/chat/src/historyBlocks.ts web/chat/src/historyBlocks.test.ts
git commit -m "feat(web): P1 友好工具名/动作短语纯函数 + 历史工具块折叠（剔除重复文本）"
```

## 任务 6：ToolCard / WorkflowCard 人话化（含 HITL「需要你确认」）

**文件：**
- 重写：`web/chat/src/components/ToolCard.tsx`；新增 `ToolCard.test.tsx`
- 重写：`web/chat/src/components/WorkflowCard.tsx`；新增 `WorkflowCard.test.tsx`
- 样式：`web/chat/src/style.css`（工具卡区块令牌化，保留现有类名，追加新样式）

**开工前必读（按真实类型适配，勿臆造字段）：**
- `web/chat/src/foldEvents.ts`：`ChatBlock` 联合里 `tool` / `workflow` 的真实字段名（`name/status/runId/arguments/result`、workflow 的 `steps` 与每步 `id/status` 的确切字段）。
- 现有 `web/chat/src/components/ToolCard.tsx`：它已导入 `resumeRun`、`parseAnalysisPageResult`、`AnalysisPagePreview`，保留这些既有能力；`resumeRun(runId, decision, comment?)` 的真实签名以 `api.ts` 为准。
- 下面给出的代码是目标形态；若真实字段名/可空性不同，**以真实类型为准调整实现与测试**，但对外文案、状态短语、HITL 行为与测试断言保持不变。

- [ ] **步骤 6.1：编写 ToolCard 失败测试（静态 node 环境）**

`ToolCard.test.tsx`：

```tsx
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { ChatBlock } from '../foldEvents'
import { ToolCard } from './ToolCard'

const catalog = [
  { name: 'query_order', title: '查询订单', description: '按单号查询' },
]

// 若真实 tool 块字段更多，按 foldEvents.ts 的真实结构补全；这里只覆盖用到的字段。
const tool = (over: Partial<Extract<ChatBlock, { kind: 'tool' }>> = {}) =>
  ({
    kind: 'tool', name: 'query_order', status: 'running', runId: 'r1', ...over,
  }) as Extract<ChatBlock, { kind: 'tool' }>

describe('ToolCard', () => {
  it('shows a friendly phrase and hides technical name when collapsed', () => {
    const html = renderToStaticMarkup(<ToolCard block={tool()} catalog={catalog} />)
    expect(html).toContain('正在处理：查询订单')
    expect(html).not.toContain('query_order')
  })
  it('falls back to technical name when not in catalog', () => {
    const html = renderToStaticMarkup(<ToolCard block={tool({ name: 'zzz' })} catalog={[]} />)
    expect(html).toContain('zzz')
  })
  it('renders 需要你确认 with 同意/拒绝 when waiting (interactive)', () => {
    const html = renderToStaticMarkup(
      <ToolCard block={tool({ status: 'waiting_human' })} catalog={catalog} />,
    )
    expect(html).toContain('需要你确认')
    expect(html).toContain('同意')
    expect(html).toContain('拒绝')
  })
  it('readOnly renders no action buttons for historical HITL', () => {
    const html = renderToStaticMarkup(
      <ToolCard block={tool({ status: 'waiting_human' })} catalog={catalog} readOnly />,
    )
    expect(html).not.toContain('同意')
    expect(html).toContain('待确认')
  })
})
```

- [ ] **步骤 6.2：重写 `ToolCard.tsx`**

目标形态（按真实字段微调）：

```tsx
import { useEffect, useState } from 'react'
import { Check, ChevronDown, Loader2, X } from 'lucide-react'
import { parseAnalysisPageResult } from '../analysisPage'
import { resumeRun } from '../api'
import { friendlyToolName, toolPhrase, type ToolCatalog } from '../friendlyTool'
import type { ChatBlock } from '../foldEvents'
import { HITL } from '../strings'
import { AnalysisPagePreview } from './AnalysisPagePreview'
import { Button } from './ui'

type ToolBlock = Extract<ChatBlock, { kind: 'tool' }>

export interface ToolCardProps {
  block: ToolBlock
  catalog?: ToolCatalog
  /** 历史回看：只渲染结果态，不出现审批操作。 */
  readOnly?: boolean
  onResumed?: () => void
}

export function ToolCard({ block, catalog = [], readOnly = false, onResumed }: ToolCardProps) {
  const waiting = block.status === 'waiting_human' && !readOnly
  const [expanded, setExpanded] = useState(waiting)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showComment, setShowComment] = useState(false)
  const [comment, setComment] = useState('')

  const analysisPage = block.result !== undefined ? parseAnalysisPageResult(block.result) : null
  const label = friendlyToolName(block.name, catalog)
  const description = catalog.find((t) => t.name === block.name)?.description?.trim() || ''

  useEffect(() => {
    if (block.status === 'waiting_human' && !readOnly) setExpanded(true)
  }, [block.status, readOnly])

  const decide = async (decision: 'approve' | 'reject') => {
    setBusy(true); setError(null)
    try {
      await resumeRun(block.runId, decision, decision === 'reject' ? comment.trim() : '')
      onResumed?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const icon =
    block.status === 'running' || block.status === 'approved' ? (
      <Loader2 size={15} className="icon-spin" aria-hidden />
    ) : block.status === 'failed' || block.status === 'rejected' ? (
      <X size={15} aria-hidden />
    ) : (
      <Check size={15} aria-hidden />
    )

  return (
    <div className={`tool-card${waiting ? ' tool-card-waiting' : ''}`}>
      {waiting ? (
        <div className="hitl-head">
          <p className="hitl-title">{HITL.title}</p>
          <p className="hitl-desc">{label}{description ? ` · ${description}` : ''}</p>
        </div>
      ) : (
        <button
          type="button"
          className="tool-card-header"
          onClick={() => setExpanded((v) => !v)}
          aria-expanded={expanded}
        >
          <span className="tool-card-phrase">
            {icon}
            {toolPhrase(block.name, block.status, catalog)}
          </span>
          <ChevronDown size={14} className={`tool-card-chevron${expanded ? ' expanded' : ''}`} aria-hidden />
        </button>
      )}

      {analysisPage && (
        <div className="tool-card-preview"><AnalysisPagePreview artifactUrl={analysisPage.artifactUrl} /></div>
      )}

      {expanded && (
        <div className="tool-card-body">
          <p className="tool-card-techname">工具：{block.name}</p>
          {block.arguments !== undefined && <pre className="tool-card-json">{formatJSON(block.arguments)}</pre>}
          {block.result !== undefined &&
            (analysisPage ? (
              <details className="tool-card-details"><summary>详情</summary>
                <pre className="tool-card-json">{formatJSON(block.result)}</pre>
              </details>
            ) : (
              <pre className="tool-card-json">{formatJSON(block.result)}</pre>
            ))}
        </div>
      )}

      {waiting && (
        <div className="tool-card-actions">
          {showComment && (
            <input
              className="hitl-comment"
              type="text"
              value={comment}
              disabled={busy}
              placeholder={HITL.commentPlaceholder}
              onChange={(e) => setComment(e.target.value)}
            />
          )}
          <Button size="sm" variant="primary" disabled={busy} onClick={() => void decide('approve')}>
            {HITL.approve}
          </Button>
          {!showComment ? (
            <Button size="sm" variant="danger" disabled={busy} onClick={() => setShowComment(true)}>
              {HITL.reject}
            </Button>
          ) : (
            <Button size="sm" variant="danger" disabled={busy} onClick={() => void decide('reject')}>
              确认拒绝
            </Button>
          )}
          <button type="button" className="tool-card-detailbtn" onClick={() => setExpanded((v) => !v)}>
            看参数
          </button>
        </div>
      )}

      {error && <p className="tool-card-error">{error}</p>}
    </div>
  )
}

function formatJSON(value: unknown): string {
  try { return JSON.stringify(value, null, 2) } catch { return String(value) }
}
```

注意只读历史态（`readOnly` 且 `waiting_human`）：走的是普通折叠分支，短语为 `待确认：查询订单`（满足测试 `toContain('待确认')`），且不渲染按钮。

- [ ] **步骤 6.3：样式令牌化并追加新类到 `style.css`**

保留现有 `.tool-card*` 类名，把其中硬编码色替换为令牌；追加：

```css
.tool-card-phrase { display: inline-flex; align-items: center; gap: 6px; }
.icon-spin { animation: baize-spin 1s linear infinite; }
@keyframes baize-spin { to { transform: rotate(360deg); } }
.tool-card-chevron { transition: transform .15s ease; }
.tool-card-chevron.expanded { transform: rotate(180deg); }
.hitl-head { margin-bottom: 6px; }
.hitl-title { margin: 0; font-weight: 600; color: var(--text); }
.hitl-desc { margin: 2px 0 0; font-size: 13px; color: var(--text-muted); }
.tool-card-techname { margin: 0 0 4px; font-size: 12px; color: var(--text-faint); }
.tool-card-actions { display: flex; flex-wrap: wrap; gap: var(--space-2); align-items: center; margin-top: 8px; }
.hitl-comment { flex: 1 1 160px; min-width: 0; padding: 6px 10px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); color: var(--text); }
.tool-card-detailbtn { border: 0; background: none; color: var(--text-muted); cursor: pointer; font-size: 12px; }
```

- [ ] **步骤 6.4：编写 WorkflowCard 失败测试并重写**

先按 `foldEvents.ts` 的真实 workflow 步状态类型调整 import 与状态字面量（pending/running/done/failed 以真实定义为准）。

`WorkflowCard.test.tsx`：

```tsx
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { ChatBlock } from '../foldEvents'
import { WorkflowCard } from './WorkflowCard'

const wf = (steps: Array<{ id: string; status: 'pending' | 'running' | 'done' | 'failed' }>) =>
  ({ kind: 'workflow', skill: 's', runId: 'r', steps }) as Extract<ChatBlock, { kind: 'workflow' }>

describe('WorkflowCard', () => {
  it('shows 第 n/共 m 步 and numbered steps without raw ids', () => {
    const html = renderToStaticMarkup(<WorkflowCard block={wf([
      { id: 'a', status: 'done' }, { id: 'b', status: 'running' }, { id: 'c', status: 'pending' },
    ])} />)
    expect(html).toContain('第 2 / 共 3 步')
    expect(html).toContain('步骤 1')
  })
})
```

重写 `WorkflowCard.tsx`（`WorkflowStepStatus` 若未从 foldEvents 导出，则用内联联合类型）：

```tsx
import { Check, Loader2, X } from 'lucide-react'
import type { ChatBlock } from '../foldEvents'

type WorkflowBlock = Extract<ChatBlock, { kind: 'workflow' }>
type StepStatus = 'pending' | 'running' | 'done' | 'failed'

export function WorkflowCard({ block }: { block: WorkflowBlock }) {
  const total = block.steps.length
  const current = block.steps.filter((s) => s.status === 'done' || s.status === 'running').length
  return (
    <div className="tool-card workflow-card">
      <div className="tool-card-header workflow-card-header">
        <span className="workflow-progress">第 {Math.max(current, 1)} / 共 {total} 步</span>
      </div>
      <ol className="workflow-steps">
        {block.steps.map((step, i) => (
          <li key={step.id} className={`workflow-step workflow-step-${step.status}`} data-status={step.status}>
            <StepIcon status={step.status as StepStatus} />
            <span>步骤 {i + 1}</span>
          </li>
        ))}
      </ol>
    </div>
  )
}

function StepIcon({ status }: { status: StepStatus }) {
  if (status === 'running') return <Loader2 size={13} className="icon-spin" aria-hidden />
  if (status === 'failed') return <X size={13} aria-hidden />
  if (status === 'done') return <Check size={13} aria-hidden />
  return <span className="workflow-step-dot" aria-hidden />
}
```

如现有 WorkflowCard 有额外 props（如 skill 名映射步骤名），保留“不暴露裸 id、默认序号步骤名”的行为；若 skill 元数据能提供中文名可作为增量，但不得让裸技术 id 出现在文案里。

- [ ] **步骤 6.5：运行与提交**

运行：`npx vitest run src/components/ToolCard.test.tsx src/components/WorkflowCard.test.tsx`；`npx tsc --noEmit`。
`ToolCard` 新增的 `catalog`/`readOnly` 都是可选 prop，现有调用方（ChatPage 实时块）无需在本任务修改即可编译；若现有调用传入了已被移除的 props（如旧的 onApproved/onReject），在本任务内把这些调用点改为新形态或保留兼容的可选 props，保证 `tsc` 通过。

```bash
git add web/chat/src/components/ToolCard.tsx web/chat/src/components/ToolCard.test.tsx web/chat/src/components/WorkflowCard.tsx web/chat/src/components/WorkflowCard.test.tsx web/chat/src/style.css
git commit -m "feat(web): P1 工具/HITL/workflow 人话化——友好步骤名、需要你确认(仅拒绝留言)、第n/m步"
```

## 任务 7：模型选择持久化（`modelChoice.ts`）+ 输入框模型芯片（`ModelChip`）

**文件：**
- 创建：`web/chat/src/modelChoice.ts` + `modelChoice.test.ts`
- 创建：`web/chat/src/components/ModelChip.tsx` + `ModelChip.test.tsx`
- 修改：`web/chat/src/components/Composer.tsx`（新增 `toolbar` 槽位）
- 修改：`web/chat/src/components/ui/DropdownMenu.tsx`（本任务需要 `disabled`，若任务 4 已加则跳过）
- 样式：`web/chat/src/style.css`

依赖：任务 1（`strings.ts` 的 `AUTO_LABEL/tierLabel/VISION_LABEL`）、任务 4（`DropdownMenu/MenuItem`）。

- [ ] **步骤 7.1：编写 `modelChoice` 失败测试**

`modelChoice.test.ts`：

```ts
// @vitest-environment jsdom
import { describe, expect, it, beforeEach } from 'vitest'
import { loadModelChoice, saveModelChoice, resolveModelChoice, MODEL_CHOICE_KEY } from './modelChoice'
import { AUTO_MODEL_ID } from './modelSelect'
import type { ModelProfile } from './api'

beforeEach(() => localStorage.clear())
const prof = (id: string): ModelProfile =>
  ({ id, name: id, model: 'm', auto_tier: 'standard', supports_vision: false }) as ModelProfile

describe('modelChoice persistence', () => {
  it('defaults to auto and round-trips a manual choice', () => {
    expect(loadModelChoice()).toBe(AUTO_MODEL_ID)
    saveModelChoice('mp_1')
    expect(localStorage.getItem(MODEL_CHOICE_KEY)).toBe('mp_1')
    expect(loadModelChoice()).toBe('mp_1')
  })
  it('saving auto stores the auto sentinel', () => {
    saveModelChoice('mp_1'); saveModelChoice(AUTO_MODEL_ID)
    expect(loadModelChoice()).toBe(AUTO_MODEL_ID)
  })
  it('falls back to auto (and flags stale) when the chosen profile is gone', () => {
    saveModelChoice('mp_gone')
    const r = resolveModelChoice(['mp_keep'].map(prof))
    expect(r.choice).toBe(AUTO_MODEL_ID)
    expect(r.stale).toBe(true)
  })
  it('keeps a still-existing manual choice', () => {
    saveModelChoice('mp_keep')
    const r = resolveModelChoice(['mp_keep'].map(prof))
    expect(r.choice).toBe('mp_keep'); expect(r.stale).toBe(false)
  })
})
```

- [ ] **步骤 7.2：实现 `modelChoice.ts`**

先确认 `AUTO_MODEL_ID` 从 `modelSelect.ts` 具名导出（任务 1 后仍在）。

```ts
import type { ModelProfile } from './api'
import { AUTO_MODEL_ID } from './modelSelect'

export const MODEL_CHOICE_KEY = 'baize.model_choice'

/** 读取持久化的模型选择；缺省/异常一律 Auto。 */
export function loadModelChoice(): string {
  try {
    return localStorage.getItem(MODEL_CHOICE_KEY)?.trim() || AUTO_MODEL_ID
  } catch {
    return AUTO_MODEL_ID
  }
}

export function saveModelChoice(id: string): void {
  try {
    localStorage.setItem(MODEL_CHOICE_KEY, id.trim() || AUTO_MODEL_ID)
  } catch {
    /* 隐私模式等：选择仍在内存态生效，只是不持久化 */
  }
}

export interface ResolvedChoice {
  choice: string
  /** 持久化的具体模型已不存在（被删），调用方应提示并回退 Auto。 */
  stale: boolean
}

/** 结合可用模型校正选择：Auto 永远有效；具体模型必须仍存在。 */
export function resolveModelChoice(profiles: Pick<ModelProfile, 'id'>[]): ResolvedChoice {
  const raw = loadModelChoice()
  if (raw === AUTO_MODEL_ID) return { choice: AUTO_MODEL_ID, stale: false }
  if (profiles.some((p) => p.id === raw)) return { choice: raw, stale: false }
  return { choice: AUTO_MODEL_ID, stale: true }
}
```

- [ ] **步骤 7.3：编写 `ModelChip` 失败测试（jsdom）**

`ModelChip.test.tsx`：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { ModelChip } from './ModelChip'
import type { ModelProfile } from '../api'
import { AUTO_MODEL_ID } from '../modelSelect'

let host: HTMLDivElement
beforeEach(() => {
  (globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div'); document.body.appendChild(host)
})
afterEach(() => host.remove())
const profiles = [
  { id: 'mp1', name: '迷你', model: 'mini', auto_tier: 'light', supports_vision: false },
  { id: 'mpv', name: '看图', model: 'v', auto_tier: 'standard', supports_vision: true },
] as ModelProfile[]
function render(value: string, onChange: (id: string) => void = () => {}) {
  act(() => { createRoot(host).render(<ModelChip profiles={profiles} value={value} onChange={onChange} />) })
}
const open = () => act(() => { (host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click() })

describe('ModelChip', () => {
  it('shows 智能选择 for auto and lists tier/vision tagged options', () => {
    render(AUTO_MODEL_ID)
    expect(host.textContent).toContain('智能选择')
    open()
    const items = host.querySelectorAll('[role="menuitem"]')
    expect(items.length).toBe(3)
    expect(host.textContent).toContain('快速')
    expect(host.textContent).toContain('能看图')
  })
  it('shows the chosen short name and emits onChange', () => {
    render('mp1', (id) => { expect(id).toBe('mp1') })
    expect(host.textContent).toContain('迷你')
    open()
    act(() => { (host.querySelectorAll('[role="menuitem"]')[1] as HTMLElement).click() })
  })
})
```

注意：芯片 trigger 复用 DropdownMenu 的 `data-testid="dropdown-trigger"`，菜单项 `role="menuitem"` 顺序为 [智能选择, mp1, mpv]。

- [ ] **步骤 7.4：实现 `ModelChip.tsx`（复用 DropdownMenu）**

```tsx
import { useMemo } from 'react'
import { Cpu } from 'lucide-react'
import type { ModelProfile } from '../api'
import { AUTO_MODEL_ID } from '../modelSelect'
import { AUTO_LABEL, tierLabel, VISION_LABEL } from '../strings'
import { DropdownMenu, type MenuItem } from './ui'

export interface ModelChipProps {
  profiles: ModelProfile[]
  value: string
  onChange: (id: string) => void
  disabled?: boolean
}

export function ModelChip({ profiles, value, onChange, disabled }: ModelChipProps) {
  const selected = value.trim() === '' ? AUTO_MODEL_ID : value
  const chosen = profiles.find((p) => p.id === selected)
  const label = chosen ? chosen.name : AUTO_LABEL

  const items: MenuItem[] = useMemo(() => {
    const autoItem: MenuItem = { id: AUTO_MODEL_ID, label: AUTO_LABEL, onSelect: () => onChange(AUTO_MODEL_ID) }
    const rest: MenuItem[] = profiles.map((p) => ({
      id: p.id,
      label: `${p.name} · ${tierLabel(p.auto_tier)}${p.supports_vision ? ` · ${VISION_LABEL}` : ''}`,
      onSelect: () => onChange(p.id),
    }))
    return [autoItem, ...rest]
  }, [profiles, onChange])

  return (
    <DropdownMenu triggerLabel="选择模型" triggerClassName="model-chip" items={items} disabled={disabled}>
      <span data-testid="model-chip" className="model-chip-inner">
        <Cpu size={14} aria-hidden /> {label}
      </span>
    </DropdownMenu>
  )
}
```

确认 `DropdownMenu` 支持 `disabled` 并转发到 trigger `<button>`（任务 4 已含：props `disabled?: boolean`，按钮 `disabled={disabled}`，onClick 在 disabled 时不打开）。若任务 4 未实现，本任务补上并在 DropdownMenu 测试加一个 disabled 用例。

- [ ] **步骤 7.5：在 `Composer.tsx` 增加芯片槽位**

`ComposerProps` 增加可选槽位（注意顶部用 `import type { ReactNode } from 'react'`，不要内联导入类型）：

```tsx
toolbar?: ReactNode
```

在 `<div className="composer-box">` 内、附件按钮 `<button className="composer-attach">` 之前插入：

```tsx
{toolbar && <span className="composer-toolbar">{toolbar}</span>}
```

不改变发送/附件/@ 补全的既有逻辑。

- [ ] **步骤 7.6：样式与校验、提交**

`style.css` 追加（只用令牌）：

```css
.composer-toolbar { display: inline-flex; align-items: center; margin-right: var(--space-1); }
.model-chip, .model-chip-inner { display: inline-flex; align-items: center; gap: 4px; }
.model-chip-inner { font-size: 13px; color: var(--text-muted); }
.model-chip { border: 1px solid var(--border); border-radius: var(--radius-pill); padding: 2px 10px; background: var(--surface); min-height: 32px; }
.model-chip:hover { background: var(--surface-2); color: var(--text); }
```

运行：`npx vitest run src/modelChoice.test.ts src/components/ModelChip.test.tsx`；`npx tsc --noEmit`。

```bash
git add web/chat/src/modelChoice.ts web/chat/src/modelChoice.test.ts web/chat/src/components/ModelChip.tsx web/chat/src/components/ModelChip.test.tsx web/chat/src/components/Composer.tsx web/chat/src/components/ui/DropdownMenu.tsx web/chat/src/style.css
git commit -m "feat(web): P1 模型芯片 + 选择 localStorage 持久化（档位快速/标准/深度思考、能看图）"
```

## 任务 8：ChatPage 总装（反馈层、工具目录、历史块、消息菜单、欢迎区、模型持久化）

**文件：**
- 修改：`web/chat/src/pages/ChatPage.tsx`
- 样式：`web/chat/src/style.css`（消息溢出菜单、欢迎区、模型芯片）
- 不新增独立测试文件：以既有 `ChatPage*` / `ChatPageModelSelect*` 测试 + `npx tsc` 覆盖；若接线改坏现有测试，同步把断言改为断言行为 / `data-testid` / role，不绑定易变中文。

**依赖：** 任务 1（strings/api/modelSelect）、2（Toast）、3（Modal/ConfirmDialog）、4（DropdownMenu/MenuItem/Button 在 `../components/ui`）、5（foldToolBlocks/ToolCatalog）、6（ToolCard/WorkflowCard 新 props）、7（ModelChip + modelChoice + Composer toolbar）。

**开工前先读 `ChatPage.tsx` 全文**，关键现状锚点（行号可能漂移，以符号为准）：
- state：`selectedModelId`、`historyPages`（约 103）、`error/setError`、`status`、`modelProfiles`、`supportsVision`、`liveRunId/liveEvents`。
- 历史事件循环 effect（约 394-416）：对每个缺失 runId 调 `listEvents(runId)`，目前只 `extractAnalysisPagesFromEvents`。
- `onNewChat`（约 420）已重置 historyPages。
- `onDeleteConversation`（约 438）现用 `window.confirm`，catch 里特判 `conversation_busy`。
- `onSend`（约 470 起）：附件门控在约 495-516（变量 `built`、`supportsVision`、`visionGate`）；成功后约 545 `setSelectedModelId('')`；catch 约 553-560 特判 `vision_unsupported`。
- 处理器：`onRollbackUser`(570)、`onRegenerate`(586)、`onRollbackTo`(617)、`onFork`(633)。
- `liveBlocks = foldEvents(...)`（约 669）；欢迎区 795-798（文案“开始对话/…粘贴临时 Token…”）；消息 map 800-848；`.msg-actions` 825-845（按钮：编辑并回滚 / 重新生成 / 回滚到此 / Fork）；实时 ToolCard 870 `<ToolCard block={block} />`、WorkflowCard 876。
- 高级区 `<details className="chat-advanced">` 909；独立 `<ModelSelect>` 在 Composer 之前（约 938-945）；`<Composer>` 约 948。
- 已导入：`listTools` 未导入；`ApiError`、`foldEvents/ChatBlock`、`ToolCard/WorkflowCard` 已导入（合并 import，勿重复）。
- API：`listTools(): Promise<ToolInfo[]>`，`ToolInfo { name; title?; description?; ... }`；`isImageAttachment` 已存在。

- [ ] **步骤 8.1：引入依赖与新状态**

按需新增/合并 import（严格模式 `noUnusedLocals`，不确定用到才导入）：

```tsx
import { GitBranch, CornerUpLeft } from 'lucide-react'
import { listTools, type ToolInfo } from '../api'
import { ToastRegion, useToast, ConfirmDialog, Modal, DropdownMenu, Button, type MenuItem } from '../components/ui'
import { ModelChip } from '../components/ModelChip'
import type { ToolCatalog } from '../friendlyTool'
import { foldToolBlocks } from '../historyBlocks'
import { loadModelChoice, saveModelChoice, resolveModelChoice } from '../modelChoice'
import { AUTO_MODEL_ID } from '../modelSelect'
import { ACTIONS, ADVANCED, WELCOME, friendlyError } from '../strings'
```

组件内新增：

```tsx
const toast = useToast()
const [toolCatalog, setToolCatalog] = useState<ToolCatalog>([])
const [historyBlocks, setHistoryBlocks] = useState<Record<string, ReturnType<typeof foldToolBlocks>>>({})
const [confirmDelete, setConfirmDelete] = useState<string | null>(null)
const [visionWarning, setVisionWarning] = useState<string | null>(null)
```

- [ ] **步骤 8.2：模型选择初始化（持久化+失效回退）与工具目录加载**

- `const [selectedModelId, setSelectedModelId] = useState('')` 改为 `useState(loadModelChoice)`。
- 在已有的加载 modelProfiles 的 effect 内，拿到 profiles 后：

```tsx
const resolved = resolveModelChoice(profiles)
if (resolved.stale) {
  setSelectedModelId(AUTO_MODEL_ID)
  saveModelChoice(AUTO_MODEL_ID)
  toast.push({ tone: 'info', title: '之前选择的模型已不可用，已切回智能选择。' })
}
```

- 新增 effect（目录缺失静默降级）：

```tsx
useEffect(() => {
  let cancelled = false
  void listTools()
    .then((tools: ToolInfo[]) => {
      if (!cancelled) setToolCatalog(tools.map((t) => ({ name: t.name, title: t.title, description: t.description })))
    })
    .catch(() => { /* 目录缺失不阻塞聊天，回退技术名 */ })
  return () => { cancelled = true }
}, [])
```

- [ ] **步骤 8.3：选择变化即持久化；删除发送后重置**

```tsx
const onChooseModel = (id: string) => { setSelectedModelId(id); saveModelChoice(id) }
```

删除 onSend 成功分支里的 `setSelectedModelId('')`（约 545）及其上方“Per-message model choice”注释。

- [ ] **步骤 8.4：历史 run 折叠工具/流程块**

在历史事件循环 effect 里，`const events = await listEvents(runId)` 之后、分析页提取之外，增加：

```tsx
const toolBlocks = foldToolBlocks(runId, events)
if (toolBlocks.length > 0) {
  setHistoryBlocks((prev) => (prev[runId] ? prev : { ...prev, [runId]: toolBlocks }))
}
```

`onNewChat` 与删除当前对话的重置处补 `setHistoryBlocks({})`。

- [ ] **步骤 8.5：错误统一走 Toast**

```tsx
const reportError = (e: unknown) => {
  const f = friendlyError(e)
  toast.push({ tone: 'error', title: f.title, detail: f.detail })
}
```

把 onSend / 删除 / HITL 等 catch 中面向用户的 `setError(技术串)` 改为 `reportError(err)`（删除 `conversation_busy`、`vision_unsupported` 的特判，friendlyError 已映射）。输入区状态行只保留运行状态 `status`，不再渲染技术 `error` 文本（若布局依赖 error 元素，保留容器但不塞技术串）。

- [ ] **步骤 8.6：图片能力拦截改 Modal**

发送前门控（约 500-505）失败分支：

```tsx
if (!gate.allowed) {
  setBusy(false)
  setVisionWarning(gate.message ?? '当前选择不能处理图片。')
  return
}
```

附件构建 catch：`setBusy(false); reportError(err); return`。发送后 catch 删除 vision 特判，统一 `reportError(err)`。

JSX 根部挂：

```tsx
<Modal open={visionWarning !== null} title="暂时无法发送图片" onClose={() => setVisionWarning(null)}
  footer={<Button variant="primary" onClick={() => setVisionWarning(null)}>知道了</Button>}>
  <p>{visionWarning}</p>
</Modal>
```

- [ ] **步骤 8.7：删除对话走 ConfirmDialog**

```tsx
const onDeleteConversation = (id: string) => setConfirmDelete(id)
const performDelete = async () => {
  const id = confirmDelete
  if (!id) return
  setConfirmDelete(null)
  try {
    await deleteConversation(id)
    toast.push({ tone: 'success', title: '对话已删除' })
  } catch (e) {
    reportError(e)
    return
  }
  if (id === conversationId) {
    stopStream(); stopPoll()
    setLiveRunId(null); setLiveEvents([]); setHistoryPages({}); setHistoryBlocks({})
    setMessages([]); setBusy(false); setStatus(''); setComposerDraft(undefined)
    setConversationId(newConversationId())
  }
  await refreshConversations()
}
```

（沿用现有删除当前会话时的重置集合，仅补 historyBlocks。）

```tsx
<ConfirmDialog open={confirmDelete !== null} danger title="删除这个对话？"
  body="将永久删除该对话的消息与相关数据，且不可恢复。" confirmText="删除"
  onConfirm={() => void performDelete()} onCancel={() => setConfirmDelete(null)} />
```

- [ ] **步骤 8.8：消息操作：常驻高频 + 「⋯」收纳低频 + 复制**

在组件内加复制 helper（兼容 HTTP 局域网无 clipboard 的非安全上下文）：

```tsx
async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) { await navigator.clipboard.writeText(text); return true }
  } catch { /* fall through */ }
  try {
    const ta = document.createElement('textarea')
    ta.value = text; ta.style.position = 'fixed'; ta.style.opacity = '0'
    document.body.appendChild(ta); ta.select()
    const ok = document.execCommand('copy'); ta.remove(); return ok
  } catch { return false }
}
const onCopyMessage = (m: ChatMessage) => {
  void copyText(m.content).then((ok) =>
    toast.push(ok ? { tone: 'success', title: '已复制' } : { tone: 'error', title: '复制失败，请手动选择文本' }))
}
```

把 `.msg-actions` 内容改为：
- user：常驻「编辑后重新回答」(ACTIONS.editAndReanswer → onRollbackUser)、「复制」(→ onCopyMessage)。
- assistant：常驻「重新回答」(ACTIONS.regenerate → onRegenerate)、「复制」。
- 三者通用：末尾一个 `DropdownMenu`（triggerLabel=ACTIONS.more）含「复制成新对话」(ACTIONS.forkAsNew，图标 `<GitBranch size={15} aria-hidden/>` → onFork)；system_note 额外在最前放「回到这里」(ACTIONS.rollbackHere，图标 `<CornerUpLeft .../>` → onRollbackTo)。

菜单项 `MenuItem[]` 按角色构造；system_note 不给常驻复制（它是系统批注）。显示条件沿用 `persisted && !busy && !liveRunId && !historyMutating`。

- [ ] **步骤 8.9：历史消息渲染工具/流程块（只读、每 run 一次）**

同文件加小工具：

```tsx
function isFirstMessageOfRun(index: number, msgs: ChatMessage[]): boolean {
  const runId = msgs[index].run_id
  if (!runId) return false
  return msgs.findIndex((mm) => mm.run_id === runId) === index
}
```

把 `messages.map((m) =>` 改为 `messages.map((m, msgIndex) =>`，在 `.msg` 气泡之前（assistant 且为该 run 首条）插入：

```tsx
{m.role === 'assistant' && m.run_id && isFirstMessageOfRun(msgIndex, messages) && historyBlocks[m.run_id] && (
  <div className="msg-history-blocks">
    {historyBlocks[m.run_id].map((b, i) =>
      b.kind === 'tool'
        ? <ToolCard key={`h-${i}`} block={b} catalog={toolCatalog} readOnly />
        : <WorkflowCard key={`h-${i}`} block={b} />,
    )}
  </div>
)}
```

实时块 `<ToolCard block={block} />`（约 870）改为 `<ToolCard block={block} catalog={toolCatalog} />`（不加 readOnly）。

- [ ] **步骤 8.10：欢迎区与高级项人话化**

欢迎区替换为：

```tsx
<div className="welcome">
  <p className="welcome-title">{WELCOME.title}</p>
  <p className="welcome-sub">{WELCOME.subtitle}</p>
</div>
```

高级区：`<summary>` 用 `{ADVANCED.summary}`；Token 标签用 `ADVANCED.tokenLabel`，其下加一行小字 `ADVANCED.tokenHint`；Webhook 标签用 `ADVANCED.webhookLabel`；移除 placeholder 里的 `Bearer eyJ…` 技术示例，改为中性占位（如空或“选填”）。不改 input 的绑定字段。

- [ ] **步骤 8.11：用 ModelChip 替换独立 ModelSelect（经 Composer toolbar）**

删除独立 `<ModelSelect .../>` 块（约 938-945），把芯片传入 Composer：

```tsx
<Composer
  disabled={composerDisabled}
  draft={composerDraft}
  skills={skills}
  onSend={(t, f) => void onSend(t, f)}
  toolbar={
    modelProfiles.length > 0 ? (
      <ModelChip profiles={modelProfiles} value={selectedModelId} onChange={onChooseModel} disabled={composerDisabled} />
    ) : (
      <Link className="model-chip model-chip-empty" to="/settings/models">添加模型</Link>
    )
  }
/>
```

若文件未导入 `Link`，加 `import { Link } from 'react-router-dom'`。若移除独立 ModelSelect 导致其 import 未使用，一并删除该 import。

- [ ] **步骤 8.12：挂 ToastRegion 并全量校验**

最外层 `<div>` 内末尾（与各 Modal 同级）挂：

```tsx
<ToastRegion toasts={toast.toasts} onDismiss={toast.dismiss} />
```

运行：`npx tsc --noEmit`；`npx vitest run`。修正受文案/结构影响的现有断言（断言 data-testid/role/行为，不绑定易变中文）。

```bash
git add web/chat/src/pages/ChatPage.tsx web/chat/src/style.css web/chat/src/components/Composer.tsx
git commit -m "feat(web): P1 聊天总装——Toast/确认框/图片警示、工具目录、历史工具步骤、消息溢出菜单、模型持久化、克制欢迎区"
```

## 任务 9：全量验收 + 重建嵌入产物 + 手动走查

**文件：** 重建 `internal/ui/dist/**`（Go `//go:embed` 产物）。本任务基本不手写业务代码，以质量门与产物重建为主。

- [ ] **步骤 9.1：前端全量质量门**

在 `web/chat` 目录运行：

```bash
npx tsc --noEmit
npx vitest run
npm run build
```

预期：tsc 零错误；全部测试通过；build 成功并把产物写入仓库 `internal/ui/dist`（Vite 已配置输出到该目录；生成新的 `index-*.js` / `index-*.css` 哈希文件，旧的同前缀哈希文件按 Vite 默认行为清理；以实际 `git status` 为准）。

- [ ] **步骤 9.2：后端不受影响**

仓库根运行：

```bash
go build ./...
go test ./internal/api/... ./internal/channel/...
```

预期：通过。本轮无 Go 逻辑改动，仅 `internal/ui/dist` 嵌入产物变化；若因嵌入资源文件名变化导致 Go 编译失败（通常 embed 是目录通配，不应发生），停下来以 BLOCKED 上报，不要自行改 Go 业务代码。

- [ ] **步骤 9.3：gofmt 自检（仅在碰到 Go 文件时）**

若除 `internal/ui/dist` 外没有任何 `.go` 改动，跳过。否则在仓库根运行 `gofmt -l .`，输出应为空；不空则 `gofmt -w` 相关文件。

- [ ] **步骤 9.4：手动走查清单（明/暗 × 桌面/手机宽度）**

实现者无需真实点完所有 UI（无浏览器自动化要求），但要在报告中逐条标注“可由现有单测覆盖 / 需要人工走查”。需要人工走查的条目留给控制者在最终验收时执行：

- 实时：普通消息；触发工具调用（友好步骤 + 详情展开技术名/JSON）；HITL（同意；拒绝→留言）；workflow（第 n/m 步）；图片（手选非视觉模型弹警示；智能选择正常发送）。
- 历史：切换对话再回来，工具/流程步骤完整且不与助手文字重复、同一 run 不重复渲染；历史 HITL 无操作按钮。
- 反馈：删除对话走自定义确认框且成功 Toast；复制成功 Toast；停服后发送看到人话 Toast + 可展开详情。
- 模型：手动选模型→发送→不回弹；刷新仍保留；删除该模型后回到聊天提示并回退智能选择。
- 消息菜单：「⋯」含复制成新对话/回到这里，点外/Esc/方向键可用。
- 欢迎区：空会话只见标题 + 一句定位，无示例/无 Token 措辞；高级项默认收起且为人话标签。
- 键盘与对比度：Tab 焦点可见、弹窗 Esc、暗色下危险按钮/正文对比度达标。

- [ ] **步骤 9.5：提交嵌入产物**

```bash
git add internal/ui/dist
git commit -m "build(web): 重建嵌入前端（P1 聊天重构）"
```

若 `npm run build` 没有产生任何 `internal/ui/dist` 差异（理论上前 8 个任务改了源码必然有差异），在报告中说明，不要制造空提交。

## 规格覆盖对照（自检）

| 规格条目 | 落地任务 |
|---|---|
| 4.1 结构化错误（主 parseJSON 抛 ApiError、保留 HTTP 前缀） | 任务 1（1.5） |
| 4.1 错误码人话 friendlyError + 未知详情 + 网络错误 | 任务 1（1.1/1.3） |
| 4.1 档位 快速/标准/深度思考、能看图、智能选择（含 ModelSettings 外溢） | 任务 1（1.6/1.7/1.8） |
| 4.2 Toast/useToast/aria-live | 任务 2 |
| 4.2 Modal 底座 | 任务 3（3.1-3.4） |
| 4.2 ConfirmDialog（危险焦点落取消） | 任务 3（3.5-3.7） |
| 4.2 DropdownMenu（点外/Esc/方向键/Enter） | 任务 4 |
| 4.3 friendlyToolName/toolPhrase 回退 | 任务 5 |
| 4.3 foldToolBlocks 历史只取工具/流程块、去文本 | 任务 5 |
| 4.3 ToolCard 友好步骤+详情+HITL 需要你确认（仅拒绝留言）+只读历史 | 任务 6 |
| 4.3 WorkflowCard 第 n/m 步、序号、不暴露裸 id | 任务 6 |
| 4.4 ModelChip（档位/能看图、空态加模型） | 任务 7 + 8.11 |
| 4.4 模型选择 localStorage 持久化/失效回退 | 任务 7 + 8.2/8.3 |
| 4.4 消息操作收纳 + 复制（含非安全上下文降级） | 8.8 |
| 4.4 删除对话 ConfirmDialog + Toast | 8.7 |
| 4.4 欢迎区克制化、高级项人话 | 8.10 |
| 4.4 图片能力不符 Modal 拦截（不自动改路由） | 8.6 |
| 错误 Toast 化、状态行去技术词 | 8.5 |
| 历史工具块接线（同 run 一次、只读） | 8.4/8.9 |
| 第 5 节 文件结构 | 全文 + 各任务文件清单 |
| 第 6 节 测试/验收 | 各任务 TDD + 任务 9 |
| 重建嵌入 dist / 后端不动 | 任务 9 |

