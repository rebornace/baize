# WebUI 体验改版 P0 地基（设计令牌 + 基础组件 + 暗色/响应式骨架）实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为 WebUI 体验改版铺好可复用地基——明/暗双主题设计令牌、拆分后的样式入口、一套不依赖组件库的基础 UI 组件、主题切换与移动端布局骨架，且不改变任何现有页面的功能与路由。

**架构：** CSS 自定义属性令牌驱动（`<html data-theme>` 切换明暗）；现有 `src/style.css` 拆为 `src/styles/*.css` 并由 `main.tsx` 统一引入；基础组件置于 `src/components/ui/`，纯展示、受控、可静态渲染测试；主题为纯前端（localStorage + 系统偏好），零后端改动。

**技术栈：** React 19、TypeScript 5.7（strict）、Vite 6、vitest 4（默认 `environment:'node'` + `renderToStaticMarkup`；交互测试在文件头加 `// @vitest-environment jsdom`，jsdom 已在 devDependencies，无需新增依赖；仅新增运行时依赖 `lucide-react`）。

**规格依据：** `docs/superpowers/specs/2026-09-09-webui-experience-refresh-design.md`（第 3、6、7、8 节）。本计划只覆盖 **P0**；P1 聊天、P2 设置 IA、P3 逐页人话化、P4 收尾各自另出计划。

**工作目录：** 所有前端命令在 `web/chat/` 下执行。

**约定：**
- 每个任务结束都要满足：`npx tsc --noEmit` 零错误、`npm test` 全绿、受影响处 `npm run build` 成功，然后按步骤给出的 message 提交。
- 测试风格遵循现状：纯逻辑/渲染用 `renderToStaticMarkup` 字符串断言；需要事件/副作用时用 jsdom 环境（文件头注释），用 `react-dom/client` 的 `createRoot` + `act`，不引入 testing-library。
- 组件统一加稳定的 `data-testid` 与 `aria-*`，断言优先面向这些钩子而非易变文案。

---

## 文件结构

**创建：**
- `web/chat/src/styles/tokens.css` — 明/暗设计令牌（CSS 变量）。
- `web/chat/src/styles/base.css` — reset、排版、全局、主题应用辅助。
- `web/chat/src/styles/components.css` — 基础组件类（按钮等）。
- `web/chat/src/styles/layout.css` — 响应式断点与布局骨架（容器/抽屉工具类）。
- `web/chat/src/theme.ts` — 主题类型、localStorage、系统偏好解析（纯函数 + 一个应用函数）。
- `web/chat/src/theme.test.ts` — 主题解析测试。
- `web/chat/src/components/ui/Button.tsx` 与 `Button.test.tsx`。
- `web/chat/src/components/ui/Card.tsx` 与 `Card.test.tsx`。
- `web/chat/src/components/ui/Badge.tsx` 与 `Badge.test.tsx`。
- `web/chat/src/components/ui/Spinner.tsx` 与 `Spinner.test.tsx`。
- `web/chat/src/components/ui/ThemeToggle.tsx` 与 `ThemeToggle.test.tsx`。
- `web/chat/src/components/ui/index.ts` — 统一导出桶文件。

**修改：**
- `web/chat/index.html` — 首屏防闪烁内联脚本（在 React 挂载前设置 `data-theme`）。
- `web/chat/src/main.tsx` — 用拆分后的样式入口替换 `import './style.css'`，挂载后应用主题。
- `web/chat/src/style.css` — 移除已迁走的令牌/基础/按钮规则，保留其余页面样式并改为引用令牌（本任务内仅做不破坏现状的最小替换，大规模迁移在后续阶段）。

**说明：** Modal/ConfirmDialog/Toast/DropdownMenu/Switch/Field 系列/Drawer/Segmented 等其余基础组件在 P1/P2 真正需要时各自随交付物引入（遵循"脚手架折进交付物"原则），本计划不提前空建。

---

## 任务 1：明/暗设计令牌与样式入口

**文件：**
- 创建：`web/chat/src/styles/tokens.css`
- 创建：`web/chat/src/styles/base.css`
- 创建：`web/chat/src/styles/components.css`
- 创建：`web/chat/src/styles/layout.css`
- 修改：`web/chat/src/main.tsx`
- 修改：`web/chat/src/style.css`（仅令牌/按钮相关最小替换，见步骤）

- [ ] **步骤 1：创建令牌文件 `styles/tokens.css`（明/暗两套）**

```css
/* 设计令牌：浅色为默认，html[data-theme="dark"] 覆盖。全站只许引用这些变量。 */
:root {
  color-scheme: light;

  /* 表面层级 */
  --bg: #f3f4f6;
  --surface: #ffffff;
  --surface-2: #f7f8fa;
  --sidebar: #ebecef;
  --border: #e2e5eb;
  --border-strong: #cfd5df;

  /* 文本 */
  --text: #1c1f26;
  --text-muted: #6b7280;
  --text-faint: #9aa1ad;

  /* 主色 */
  --accent: #2563eb;
  --accent-hover: #1d4ed8;
  --accent-active: #1e40af;
  --accent-soft: rgba(37, 99, 235, 0.10);
  --on-accent: #ffffff;

  /* 语义色 */
  --success: #16a34a;
  --success-soft: #dcfce7;
  --warning: #d97706;
  --warning-soft: #fff4e0;
  --danger: #dc2626;
  --danger-soft: rgba(220, 38, 38, 0.10);
  --info: #2563eb;
  --info-soft: #e8f0ff;

  /* 聊天专用（保留兼容） */
  --user-bubble: #e8eaed;
  --assistant-bubble: transparent;
  --system-bubble: #f8fafc;
  --hitl-border: var(--warning);
  --hitl-bg: var(--warning-soft);

  /* 形状 */
  --radius-sm: 8px;
  --radius: 10px;
  --radius-lg: 14px;
  --radius-xl: 16px;
  --radius-pill: 999px;

  /* 间距（4px 基准） */
  --space-1: 4px;
  --space-2: 8px;
  --space-3: 12px;
  --space-4: 16px;
  --space-6: 24px;
  --space-8: 32px;

  /* 阴影 */
  --shadow-1: 0 1px 2px rgba(16, 24, 40, 0.06);
  --shadow-2: 0 4px 12px rgba(16, 24, 40, 0.10);
  --shadow-3: 0 12px 32px rgba(16, 24, 40, 0.16);

  /* 字体 / 动效 / 层级 */
  --font-sans: "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
  --chat-max: 48rem;
  --t-fast: 120ms ease;
  --t: 180ms ease;
  --z-drawer: 1000;
  --z-modal: 1100;
  --z-toast: 1200;
}

html[data-theme="dark"] {
  color-scheme: dark;

  --bg: #15181e;
  --surface: #1c2027;
  --surface-2: #232833;
  --sidebar: #1a1e25;
  --border: #2c323d;
  --border-strong: #3a4150;

  --text: #e6e9ef;
  --text-muted: #a0a7b4;
  --text-faint: #6b7280;

  --accent: #3b82f6;
  --accent-hover: #60a5fa;
  --accent-active: #2563eb;
  --accent-soft: rgba(59, 130, 246, 0.18);
  --on-accent: #ffffff;

  --success: #22c55e;
  --success-soft: rgba(34, 197, 94, 0.16);
  --warning: #f59e0b;
  --warning-soft: rgba(245, 158, 11, 0.16);
  --danger: #f87171;
  --danger-soft: rgba(248, 113, 113, 0.16);
  --info: #60a5fa;
  --info-soft: rgba(96, 165, 250, 0.16);

  --user-bubble: #2a303c;
  --assistant-bubble: transparent;
  --system-bubble: #20242d;

  --shadow-1: 0 1px 2px rgba(0, 0, 0, 0.4);
  --shadow-2: 0 4px 12px rgba(0, 0, 0, 0.45);
  --shadow-3: 0 12px 32px rgba(0, 0, 0, 0.55);
}
```

- [ ] **步骤 2：创建 `styles/base.css`（reset/排版/焦点可见）**

```css
*,
*::before,
*::after {
  box-sizing: border-box;
}

html,
body,
#app {
  margin: 0;
  height: 100%;
}

body {
  background: var(--bg);
  color: var(--text);
  font-family: var(--font-sans);
  line-height: 1.55;
  transition: background var(--t), color var(--t);
}

:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

button,
input,
select,
textarea {
  font: inherit;
}
```

- [ ] **步骤 3：创建 `styles/components.css`（按钮令牌化，作为新组件类的基线）**

```css
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  padding: 8px 14px;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: background var(--t-fast), color var(--t-fast), border-color var(--t-fast);
}

.btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.btn.primary {
  background: var(--accent);
  color: var(--on-accent);
}
.btn.primary:hover:not(:disabled) {
  background: var(--accent-hover);
}

.btn.secondary {
  background: var(--surface);
  color: var(--text);
  border-color: var(--border-strong);
}
.btn.secondary:hover:not(:disabled) {
  background: var(--surface-2);
}

.btn.danger {
  background: var(--danger);
  color: #fff;
}
.btn.ghost {
  background: transparent;
  color: var(--text-muted);
}
.btn.ghost:hover:not(:disabled) {
  background: var(--surface-2);
  color: var(--text);
}

.btn.sm {
  padding: 5px 10px;
  font-size: 0.8125rem;
}
```

- [ ] **步骤 4：创建 `styles/layout.css`（响应式断点与抽屉工具类）**

```css
/* 移动端：≤768；平板：769–1024；桌面：>1024 */
.app-drawer-scrim {
  display: none;
}

@media (max-width: 1024px) {
  :root {
    --chat-max: 100%;
  }
}

@media (max-width: 768px) {
  .app-with-drawer .chat-sidebar,
  .app-with-drawer .settings-nav {
    position: fixed;
    inset: 0 auto 0 0;
    width: min(82vw, 320px);
    z-index: var(--z-drawer);
    transform: translateX(-100%);
    transition: transform var(--t);
    box-shadow: var(--shadow-3);
  }
  .app-with-drawer.drawer-open .chat-sidebar,
  .app-with-drawer.drawer-open .settings-nav {
    transform: translateX(0);
  }
  .app-with-drawer.drawer-open .app-drawer-scrim {
    display: block;
    position: fixed;
    inset: 0;
    z-index: calc(var(--z-drawer) - 1);
    background: rgba(0, 0, 0, 0.4);
    border: none;
  }
}
```

- [ ] **步骤 5：在 `main.tsx` 顶部按顺序引入样式**

把现有 `import './style.css'` 替换为：

```ts
import './styles/tokens.css'
import './styles/base.css'
import './styles/components.css'
import './styles/layout.css'
import './style.css'
```

`./style.css` 保留在最后，使尚未迁移的页面规则继续生效；新令牌在其之前可用。

- [ ] **步骤 6：最小替换 `style.css` 中会冲突的重复定义**

删除 `style.css` 顶部旧的 `:root { ... }` 令牌块与 `body { ... }` 基础块（约第 1–34 行，已迁入 tokens/base），并把旧变量名在整个 `style.css` 内做等价替换，保证现有页面不破：
- `var(--panel)` → `var(--surface)`；`var(--muted)` → `var(--text-muted)`；`var(--sidebar)`/`var(--border)`/`var(--text)`/`var(--accent)`/`var(--danger)` 保持不变（tokens 已定义同名）。
- 旧 `.btn` 块（约 792–825 行）删除（components.css 已提供且变体更全）。
- `--user`/`--assistant`/`--system` 的引用改为 `--user-bubble`/`--assistant-bubble`/`--system-bubble`；`--hitl-border`/`--hitl-bg` 名称未变，无需改。

- [ ] **步骤 7：验证与提交**

运行：`npx tsc --noEmit`（预期零错误）、`npm test`（预期全绿）、`npm run build`（预期成功）。
肉眼验证：`npm run dev` 后聊天页布局/配色与改版前一致（令牌等价替换）。

```bash
git add web/chat/src/styles web/chat/src/main.tsx web/chat/src/style.css
git commit -m "feat(web): 设计令牌（明暗）+ 拆分样式入口 + 响应式断点骨架"
```

---

## 任务 2：主题解析（纯函数 TDD）+ 首屏防闪烁

**文件：**
- 创建：`web/chat/src/theme.ts`
- 测试：`web/chat/src/theme.test.ts`
- 修改：`web/chat/index.html`

- [ ] **步骤 1：编写失败测试 `theme.test.ts`**

```ts
import { describe, expect, it } from 'vitest'
import {
  THEME_KEY,
  type ThemeChoice,
  applyTheme,
  resolveTheme,
  systemTheme,
} from './theme'

describe('resolveTheme', () => {
  it('returns the stored choice when it is light or dark', () => {
    expect(resolveTheme('light', () => 'dark')).toBe('light')
    expect(resolveTheme('dark', () => 'light')).toBe('dark')
  })

  it('falls back to the OS preference for empty/unknown stored values', () => {
    expect(resolveTheme('', () => 'dark')).toBe('dark')
    expect(resolveTheme('garbage', () => 'light')).toBe('light')
  })
})

describe('systemTheme', () => {
  it('normalizes an unmatched media query to light', () => {
    expect(systemTheme(false)).toBe('light')
    expect(systemTheme(true)).toBe('dark')
  })
})

describe('applyTheme', () => {
  it('sets data-theme on the document root', () => {
    const calls: Array<[string, string]> = []
    const root = {
      setAttribute: (name: string, value: string) => calls.push([name, value]),
    }
    applyTheme('dark', root as unknown as HTMLElement)
    expect(calls).toEqual([['data-theme', 'dark']])
  })
})

describe('THEME_KEY', () => {
  it('uses the stable localStorage key', () => {
    expect(THEME_KEY).toBe('baize.theme')
  })
})

// 类型层保证：ThemeChoice 只能是这三者
const _choices: ThemeChoice[] = ['light', 'dark', 'system']
void _choices
```

- [ ] **步骤 2：运行测试确认失败**

运行：`npx vitest run src/theme.test.ts`
预期：FAIL，报错无法解析 `./theme`（模块不存在）。

- [ ] **步骤 3：实现 `theme.ts`（最少代码）**

```ts
// 主题：纯前端。用户可选 light/dark/system；system 跟随 prefers-color-scheme。
export type ThemeMode = 'light' | 'dark'
export type ThemeChoice = ThemeMode | 'system'

export const THEME_KEY = 'baize.theme'

/** 读取本地存储的选择；SSR/无 localStorage 时返回空串。 */
export function readStoredTheme(storage: Storage | undefined): string {
  try {
    return storage?.getItem(THEME_KEY)?.trim() ?? ''
  } catch {
    return ''
  }
}

/** 由系统暗色媒体查询是否命中，返回明暗。 */
export function systemTheme(prefersDark: boolean): ThemeMode {
  return prefersDark ? 'dark' : 'light'
}

/** 存储值合法直接采用；否则（空/未知）回退到系统偏好。 */
export function resolveTheme(stored: string, prefersDark: () => boolean): ThemeMode {
  if (stored === 'light' || stored === 'dark') return stored
  return systemTheme(prefersDark())
}

/** 把最终明暗写到 <html data-theme>。抽出 root 参数便于在 node 环境测试。 */
export function applyTheme(mode: ThemeMode, root: HTMLElement): void {
  root.setAttribute('data-theme', mode)
}
```

- [ ] **步骤 4：运行测试确认通过**

运行：`npx vitest run src/theme.test.ts`
预期：PASS（全部用例）。

- [ ] **步骤 5：`index.html` 加首屏防闪烁内联脚本**

在 `<head>` 内、`<title>` 之前加入（必须在首帧前执行，避免明暗闪烁 FOUC）：

```html
    <script>
      (function () {
        try {
          var stored = (localStorage.getItem('baize.theme') || '').trim();
          var dark =
            stored === 'dark' ||
            (stored !== 'light' &&
              window.matchMedia('(prefers-color-scheme: dark)').matches);
          document.documentElement.setAttribute('data-theme', dark ? 'dark' : 'light');
        } catch (e) {
          document.documentElement.setAttribute('data-theme', 'light');
        }
      })();
    </script>
```

- [ ] **步骤 6：验证与提交**

运行：`npx tsc --noEmit`、`npm test`、`npm run build`，均应通过；`npm run dev` 在系统暗色模式下首屏即暗色，手动改 localStorage 后刷新不闪烁。

```bash
git add web/chat/src/theme.ts web/chat/src/theme.test.ts web/chat/index.html
git commit -m "feat(web): 主题解析纯函数 + 首屏防闪烁（跟随系统/localStorage）"
```

---

## 任务 3：基础组件 Button / Card / Badge / Spinner

**文件：**
- 创建：`web/chat/src/components/ui/Button.tsx`、`Button.test.tsx`
- 创建：`web/chat/src/components/ui/Card.tsx`、`Card.test.tsx`
- 创建：`web/chat/src/components/ui/Badge.tsx`、`Badge.test.tsx`
- 创建：`web/chat/src/components/ui/Spinner.tsx`、`Spinner.test.tsx`
- 创建：`web/chat/src/components/ui/index.ts`

这些组件纯展示、透传原生属性、可 `renderToStaticMarkup` 测试（沿用默认 node 环境，不需要 jsdom）。

- [ ] **步骤 1：为 Button 编写失败测试 `Button.test.tsx`**

```tsx
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { Button } from './Button'

describe('Button', () => {
  it('renders a primary button with testid and children', () => {
    const html = renderToStaticMarkup(<Button variant="primary">保存</Button>)
    expect(html).toContain('data-testid="ui-button"')
    expect(html).toContain('btn primary')
    expect(html).toContain('保存')
  })

  it('supports secondary/ghost/danger variants and sm size', () => {
    expect(renderToStaticMarkup(<Button variant="secondary" />)).toContain('btn secondary')
    expect(renderToStaticMarkup(<Button variant="ghost" />)).toContain('btn ghost')
    expect(renderToStaticMarkup(<Button variant="danger" />)).toContain('btn danger')
    expect(renderToStaticMarkup(<Button size="sm" />)).toContain('btn sm')
  })

  it('renders native type and disabled, and merges extra class', () => {
    const html = renderToStaticMarkup(
      <Button type="submit" disabled className="extra" />,
    )
    expect(html).toContain('type="submit"')
    expect(html).toContain('disabled=""')
    expect(html).toContain('extra')
  })
})
```

- [ ] **步骤 2：运行确认失败，再实现 `Button.tsx`**

运行：`npx vitest run src/components/ui/Button.test.tsx`，预期 FAIL（模块不存在）。

```tsx
import type { ButtonHTMLAttributes, ReactNode } from 'react'

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'
export type ButtonSize = 'md' | 'sm'

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: ButtonSize
  children?: ReactNode
}

export function Button({
  variant = 'secondary',
  size = 'md',
  className = '',
  type = 'button',
  children,
  ...rest
}: ButtonProps) {
  const classes = ['btn', variant, size === 'sm' ? 'sm' : '', className]
    .filter(Boolean)
    .join(' ')
  return (
    <button type={type} className={classes} data-testid="ui-button" {...rest}>
      {children}
    </button>
  )
}
```

运行同一测试，预期 PASS。

- [ ] **步骤 3：以同样 TDD 节奏实现 Badge（先写失败测试）**

`Badge.test.tsx`：

```tsx
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { Badge } from './Badge'

describe('Badge', () => {
  it('renders neutral tone by default', () => {
    const html = renderToStaticMarkup(<Badge>已开启</Badge>)
    expect(html).toContain('data-testid="ui-badge"')
    expect(html).toContain('ui-badge neutral')
    expect(html).toContain('已开启')
  })

  it.each(['success', 'warning', 'danger', 'info'] as const)('supports tone %s', (tone) => {
    expect(renderToStaticMarkup(<Badge tone={tone} />)).toContain(`ui-badge ${tone}`)
  })
})
```

`Badge.tsx`：

```tsx
import type { HTMLAttributes, ReactNode } from 'react'

export type BadgeTone = 'neutral' | 'success' | 'warning' | 'danger' | 'info'

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: BadgeTone
  children?: ReactNode
}

export function Badge({ tone = 'neutral', className = '', children, ...rest }: BadgeProps) {
  return (
    <span
      className={`ui-badge ${tone} ${className}`.trim()}
      data-testid="ui-badge"
      {...rest}
    >
      {children}
    </span>
  )
}
```

向 `styles/components.css` 追加徽章样式：

```css
.ui-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 0.75rem;
  line-height: 1;
  padding: 3px 8px;
  border-radius: var(--radius-pill);
  background: var(--surface-2);
  color: var(--text-muted);
  border: 1px solid var(--border);
  white-space: nowrap;
}
.ui-badge.success { background: var(--success-soft); color: var(--success); border-color: transparent; }
.ui-badge.warning { background: var(--warning-soft); color: var(--warning); border-color: transparent; }
.ui-badge.danger  { background: var(--danger-soft);  color: var(--danger);  border-color: transparent; }
.ui-badge.info    { background: var(--info-soft);    color: var(--info);    border-color: transparent; }
```

- [ ] **步骤 4：TDD 实现 Card**

`Card.test.tsx` 断言：含 `data-testid="ui-card"`、类 `ui-card`、渲染标题/正文/子节点、合并自定义类。

```tsx
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { Card } from './Card'

describe('Card', () => {
  it('renders title, description and body content', () => {
    const html = renderToStaticMarkup(
      <Card title="AI 模型" description="选择对话用的大脑">
        <span>body</span>
      </Card>,
    )
    expect(html).toContain('data-testid="ui-card"')
    expect(html).toContain('AI 模型')
    expect(html).toContain('选择对话用的大脑')
    expect(html).toContain('body')
  })
})
```

`Card.tsx`：

```tsx
import type { ReactNode } from 'react'

export interface CardProps {
  title?: ReactNode
  description?: ReactNode
  icon?: ReactNode
  /** 右上角附加内容（如状态徽标或箭头）。 */
  trailing?: ReactNode
  className?: string
  onClick?: () => void
  children?: ReactNode
}

export function Card({
  title,
  description,
  icon,
  trailing,
  className = '',
  onClick,
  children,
  ...rest
}: CardProps) {
  const clickable = onClick ? ' ui-card-clickable' : ''
  return (
    <div
      className={`ui-card${clickable} ${className}`.trim()}
      data-testid="ui-card"
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      {...rest}
    >
      {(title || trailing) && (
        <div className="ui-card-head">
          <div className="ui-card-title">
            {icon && <span className="ui-card-icon">{icon}</span>}
            {title}
          </div>
          {trailing}
        </div>
      )}
      {description && <p className="ui-card-desc">{description}</p>}
      {children}
    </div>
  )
}
```

向 `styles/components.css` 追加：

```css
.ui-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: var(--space-4);
  box-shadow: var(--shadow-1);
}
.ui-card-clickable { cursor: pointer; transition: border-color var(--t-fast), box-shadow var(--t-fast); }
.ui-card-clickable:hover { border-color: var(--border-strong); box-shadow: var(--shadow-2); }
.ui-card-clickable:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.ui-card-head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-2); }
.ui-card-title { display: flex; align-items: center; gap: var(--space-2); font-weight: 600; color: var(--text); }
.ui-card-icon { display: inline-flex; color: var(--accent); }
.ui-card-desc { margin: 6px 0 0; font-size: 0.8125rem; color: var(--text-muted); }
```

- [ ] **步骤 5：TDD 实现 Spinner**

`Spinner.test.tsx`：

```tsx
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { Spinner } from './Spinner'

describe('Spinner', () => {
  it('renders an accessible spinner', () => {
    const html = renderToStaticMarkup(<Spinner label="加载中" />)
    expect(html).toContain('data-testid="ui-spinner"')
    expect(html).toContain('role="status"')
    expect(html).toContain('加载中')
  })
})
```

`Spinner.tsx`：

```tsx
export interface SpinnerProps {
  label?: string
  className?: string
}

export function Spinner({ label, className = '' }: SpinnerProps) {
  return (
    <span className={`ui-spinner ${className}`.trim()} role="status" aria-label={label} data-testid="ui-spinner">
      <span className="ui-spinner-dot" aria-hidden="true" />
      {label && <span className="ui-spinner-label">{label}</span>}
    </span>
  )
}
```

向 `styles/components.css` 追加：

```css
.ui-spinner { display: inline-flex; align-items: center; gap: var(--space-2); color: var(--text-muted); font-size: 0.8125rem; }
.ui-spinner-dot {
  width: 14px; height: 14px; border-radius: 50%;
  border: 2px solid var(--border-strong);
  border-top-color: var(--accent);
  display: inline-block; animation: ui-spin 0.7s linear infinite;
}
.ui-spinner-label { line-height: 1; }
@keyframes ui-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .ui-spinner-dot { animation: none; } }
```

- [ ] **步骤 6：创建桶导出 `components/ui/index.ts`**

```ts
export { Button, type ButtonProps, type ButtonVariant, type ButtonSize } from './Button'
export { Card, type CardProps } from './Card'
export { Badge, type BadgeProps, type BadgeTone } from './Badge'
export { Spinner, type SpinnerProps } from './Spinner'
```

- [ ] **步骤 7：验证与提交**

运行：`npx tsc --noEmit`、`npm test`（含新增 4 个组件测试，全绿）、`npm run build`。

```bash
git add web/chat/src/components/ui web/chat/src/styles/components.css
git commit -m "feat(web): 基础组件 Button/Card/Badge/Spinner（令牌化、可测试）"
```

---

## 任务 4：`useTheme` 钩子 + 主题切换控件（接入聊天与设置外壳）

**文件：**
- 创建：`web/chat/src/useTheme.ts`
- 创建：`web/chat/src/components/ui/ThemeToggle.tsx`、`ThemeToggle.test.tsx`（jsdom）
- 修改：`web/chat/src/pages/ChatPage.tsx`（侧栏底部插入）
- 修改：`web/chat/src/pages/SettingsLayout.tsx`（导航底部插入）
- 修改：`web/chat/src/components/ui/index.ts`
- 修改：`web/chat/package.json`、`package-lock.json`（新增 `lucide-react`）
- 样式：向 `styles/components.css` 追加分段控件样式

- [ ] **步骤 1：安装图标库**

运行（在 `web/chat/`）：`npm install lucide-react`
预期：`package.json` dependencies 增加 `lucide-react`，仅打包用到的图标。

- [ ] **步骤 2：实现 `useTheme.ts`**

```ts
import { useCallback, useEffect, useState } from 'react'
import {
  THEME_KEY,
  applyTheme,
  readStoredTheme,
  resolveTheme,
  type ThemeChoice,
} from './theme'

function prefersDark(): boolean {
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

function isChoice(raw: string): raw is ThemeChoice {
  return raw === 'light' || raw === 'dark' || raw === 'system'
}

/** 主题选择的唯一状态入口：持久化 + 应用 + 跟随系统变化。 */
export function useTheme() {
  const [choice, setChoiceState] = useState<ThemeChoice>(() => {
    const raw = readStoredTheme(
      typeof localStorage === 'undefined' ? undefined : localStorage,
    )
    return isChoice(raw) ? raw : 'system'
  })

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const apply = () =>
      applyTheme(
        resolveTheme(readStoredTheme(localStorage), () => mq.matches),
        document.documentElement,
      )
    apply()
    mq.addEventListener('change', apply)
    return () => mq.removeEventListener('change', apply)
  }, [choice])

  const setChoice = useCallback((next: ThemeChoice) => {
    try {
      localStorage.setItem(THEME_KEY, next)
    } catch {
      /* 隐私模式等：本次仍生效，只是不持久化 */
    }
    applyTheme(resolveTheme(next, prefersDark), document.documentElement)
    setChoiceState(next)
  }, [])

  return { choice, setChoice }
}
```

- [ ] **步骤 3：先写 jsdom 失败测试 `ThemeToggle.test.tsx`**

文件首行必须声明 jsdom 环境（覆盖默认 node）：

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { ThemeToggle } from './ThemeToggle'

function installMatchMedia(initialDark: boolean) {
  const listeners = new Set<() => void>()
  const mql = {
    matches: initialDark,
    addEventListener: (_e: string, fn: () => void) => listeners.add(fn),
    removeEventListener: (_e: string, fn: () => void) => listeners.delete(fn),
  }
  window.matchMedia = (() => mql) as unknown as typeof window.matchMedia
}

let container: HTMLDivElement
beforeEach(() => {
  globalThis.IS_REACT_ACT_ENVIRONMENT = true
  localStorage.clear()
  installMatchMedia(false)
  container = document.createElement('div')
  document.body.appendChild(container)
})
afterEach(() => {
  container.remove()
})

function render() {
  act(() => {
    createRoot(container).render(<ThemeToggle />)
  })
}

describe('ThemeToggle', () => {
  it('applies and persists dark when the dark button is pressed', () => {
    render()
    const btn = container.querySelector('[aria-label="深色"]') as HTMLButtonElement
    act(() => btn.click())
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    expect(localStorage.getItem('baize.theme')).toBe('dark')
    expect(btn.getAttribute('aria-pressed')).toBe('true')
  })

  it('applies light and keeps the other options unpressed', () => {
    localStorage.setItem('baize.theme', 'dark')
    render()
    const light = container.querySelector('[aria-label="浅色"]') as HTMLButtonElement
    act(() => light.click())
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    expect(light.getAttribute('aria-pressed')).toBe('true')
    expect(
      (container.querySelector('[aria-label="跟随系统"]') as HTMLButtonElement).getAttribute(
        'aria-pressed',
      ),
    ).toBe('false')
  })
})
```

运行：`npx vitest run src/components/ui/ThemeToggle.test.tsx`，预期 FAIL（模块不存在）。

- [ ] **步骤 4：实现 `ThemeToggle.tsx`**

```tsx
import { Monitor, Moon, Sun } from 'lucide-react'
import { useTheme } from '../../useTheme'
import type { ThemeChoice } from '../../theme'

const OPTIONS: Array<{ value: ThemeChoice; label: string; Icon: typeof Sun }> = [
  { value: 'light', label: '浅色', Icon: Sun },
  { value: 'system', label: '跟随系统', Icon: Monitor },
  { value: 'dark', label: '深色', Icon: Moon },
]

export function ThemeToggle() {
  const { choice, setChoice } = useTheme()
  return (
    <div className="ui-theme-toggle" role="group" aria-label="主题外观" data-testid="ui-theme-toggle">
      {OPTIONS.map(({ value, label, Icon }) => (
        <button
          key={value}
          type="button"
          className="ui-theme-btn"
          aria-label={label}
          aria-pressed={choice === value}
          onClick={() => setChoice(value)}
        >
          <Icon size={15} aria-hidden="true" />
        </button>
      ))}
    </div>
  )
}
```

向 `styles/components.css` 追加：

```css
.ui-theme-toggle {
  display: inline-flex;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 2px;
  background: var(--surface-2);
}
.ui-theme-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
}
.ui-theme-btn:hover { color: var(--text); }
.ui-theme-btn[aria-pressed="true"] {
  background: var(--surface);
  color: var(--accent);
  box-shadow: var(--shadow-1);
}
```

并在 `components/ui/index.ts` 追加：`export { ThemeToggle } from './ThemeToggle'`。

运行同一测试，预期 PASS。

- [ ] **步骤 5：接入两个外壳**

- `ChatPage.tsx`：顶部 import `import { ThemeToggle } from '../components/ui'`；在侧栏底部 `chat-sidebar-bottom` 内、设置链接与「退出」按钮同层加入 `<ThemeToggle />`（放在设置链接上方即可）。
- `SettingsLayout.tsx`：同样 import，并在 `<Link to="/" className="settings-back">返回聊天</Link>` 上方加入 `<ThemeToggle />`。
- 两处均不改业务逻辑；现有测试不渲染这两个整页，不应受影响。

- [ ] **步骤 6：验证与提交**

运行：`npx tsc --noEmit`、`npm test`、`npm run build`，均通过。

```bash
git add web/chat/package.json web/chat/package-lock.json web/chat/src/useTheme.ts web/chat/src/components/ui web/chat/src/styles/components.css web/chat/src/pages/ChatPage.tsx web/chat/src/pages/SettingsLayout.tsx
git commit -m "feat(web): 主题切换控件（浅/跟随系统/深）接入聊天与设置外壳"
```

---

## 任务 5：移动端抽屉导航（聊天与设置外壳）

**文件：**
- 创建：`web/chat/src/useDrawer.ts`、`useDrawer.test.tsx`（jsdom）
- 修改：`web/chat/src/pages/ChatPage.tsx`
- 修改：`web/chat/src/pages/SettingsLayout.tsx`
- 样式：向 `styles/layout.css` 追加汉堡按钮/移动顶栏

- [ ] **步骤 1：先写失败测试 `useDrawer.test.tsx`**

```tsx
// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { useDrawer } from './useDrawer'

let container: HTMLDivElement
beforeEach(() => {
  globalThis.IS_REACT_ACT_ENVIRONMENT = true
  container = document.createElement('div')
  document.body.appendChild(container)
})
afterEach(() => container.remove())

function Harness() {
  const d = useDrawer()
  return (
    <div>
      <span data-testid="open">{String(d.open)}</span>
      <button data-testid="open-btn" onClick={d.open}>open</button>
      <button data-testid="close-btn" onClick={d.close}>close</button>
      <button data-testid="toggle-btn" onClick={d.toggle}>toggle</button>
    </div>
  )
}

function render() {
  act(() => createRoot(container).render(<Harness />))
}
const text = () =>
  (container.querySelector('[data-testid="open"]') as HTMLElement).textContent

describe('useDrawer', () => {
  it('starts closed and opens/closes/toggles', () => {
    render()
    expect(text()).toBe('false')
    act(() => (container.querySelector('[data-testid="open-btn"]') as HTMLButtonElement).click())
    expect(text()).toBe('true')
    act(() => (container.querySelector('[data-testid="close-btn"]') as HTMLButtonElement).click())
    expect(text()).toBe('false')
    act(() => (container.querySelector('[data-testid="toggle-btn"]') as HTMLButtonElement).click())
    expect(text()).toBe('true')
  })
})
```

运行：`npx vitest run src/useDrawer.test.tsx`，预期 FAIL（模块不存在）。

- [ ] **步骤 2：实现 `useDrawer.ts`**

```ts
import { useCallback, useState } from 'react'

/** 移动端抽屉的开关状态；初始关闭。 */
export function useDrawer(initial = false) {
  const [open, setOpen] = useState(initial)
  return {
    open,
    open: useCallback(() => setOpen(true), []),
    close: useCallback(() => setOpen(false), []),
    toggle: useCallback(() => setOpen((v) => !v), []),
  }
}
```

运行同一测试，预期 PASS。

- [ ] **步骤 3：向 `styles/layout.css` 追加汉堡按钮与移动顶栏**

```css
.app-menu-btn {
  display: none;
}
.app-mobile-bar {
  display: none;
}

@media (max-width: 768px) {
  .app-menu-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    border: none;
    background: transparent;
    color: var(--text);
    cursor: pointer;
  }
  .app-mobile-bar {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }
  .chat-main,
  .settings-main { min-width: 0; }
  .messages-inner { padding-left: var(--space-3); padding-right: var(--space-3); }
}
```

- [ ] **步骤 4：接入 `ChatPage.tsx`**

- import：`import { Menu } from 'lucide-react'`、`import { useDrawer } from '../useDrawer'`。
- 组件内：`const drawer = useDrawer()`。
- 根节点改为：

```tsx
<div className={`chat-shell app-with-drawer${drawer.open ? ' drawer-open' : ''}`}>
  <button
    type="button"
    className="app-drawer-scrim"
    aria-label="关闭菜单"
    onClick={drawer.close}
  />
  <aside className="chat-sidebar" aria-label="对话列表">
```

- 在 `<main className="chat-main">` 内、消息滚动区之前插入移动顶栏：

```tsx
<div className="app-mobile-bar">
  <button type="button" className="app-menu-btn" aria-label="打开对话列表" onClick={drawer.open}>
    <Menu size={20} aria-hidden="true" />
  </button>
</div>
```

- 在选择对话与新建对话的处理函数（`onSelectConversation`、`onNewChat`）成功后调用 `drawer.close()`（仅收起菜单，不改既有逻辑）。

- [ ] **步骤 5：接入 `SettingsLayout.tsx`**

```tsx
import { Menu } from 'lucide-react'
import { useDrawer } from '../useDrawer'
// ...
export function SettingsLayout() {
  const { role } = useGate()
  const nav = settingsNavItems(role)
  const drawer = useDrawer()
  return (
    <div className={`settings-shell app-with-drawer${drawer.open ? ' drawer-open' : ''}`}>
      <button type="button" className="app-drawer-scrim" aria-label="关闭菜单" onClick={drawer.close} />
      <aside className="settings-nav" aria-label="设置导航">
        {/* 原有内容不变 */}
      </aside>
      <main className="settings-main">
        <div className="app-mobile-bar">
          <button type="button" className="app-menu-btn" aria-label="打开设置菜单" onClick={drawer.open}>
            <Menu size={20} aria-hidden="true" />
          </button>
          <strong>设置</strong>
        </div>
        <Outlet />
      </main>
    </div>
  )
}
```

（保留原有标题、`nav.map`、主题控件、「返回聊天」链接；仅新增包裹类、scrim、移动顶栏与 `useDrawer`。）

- [ ] **步骤 6：验证与提交**

运行：`npx tsc --noEmit`、`npm test`、`npm run build`。
手动走查（`npm run dev`，DevTools 切到 ≤768px）：汉堡出现 → 点击侧栏滑入、遮罩出现 → 点遮罩/选对话后收起；桌面宽度下汉堡与遮罩均不可见、布局不变。

```bash
git add web/chat/src/useDrawer.ts web/chat/src/useDrawer.test.tsx web/chat/src/styles/layout.css web/chat/src/pages/ChatPage.tsx web/chat/src/pages/SettingsLayout.tsx
git commit -m "feat(web): 移动端抽屉导航（聊天/设置外壳，≤768px 抽屉+遮罩）"
```

---

## 任务 6：重建嵌入产物并做 P0 总验收

**文件：**
- 重建产物：`internal/ui/dist/**`（Go `//go:embed`）

- [ ] **步骤 1：全量前端校验**

在 `web/chat/` 运行：`npx tsc --noEmit`、`npm test`、`npm run build`。
预期：零类型错误、全部单测通过、构建成功并刷新 `../../internal/ui/dist`。

- [ ] **步骤 2：确认后端仍可编译/测试（仅嵌入产物变化）**

在仓库根运行：`go build ./...` 与 `go test ./internal/api/... ./cmd/baize/...`。
预期：通过（本阶段不改 Go 逻辑，仅嵌入资源变化）。

- [ ] **步骤 3：核对暗色与响应式无硬编码遗漏（人工）**

- 全局搜索新增样式，确保颜色只引用令牌：`rg -n "#[0-9a-fA-F]{3,6}" web/chat/src/styles` 中除令牌定义文件 `tokens.css` 外应为空。
- DevTools 逐一切换：浅色/深色 × 桌面/平板/手机，确认聊天页与设置页在 P0 范围内（外壳、按钮、卡片、徽章）显示正常、无横向滚动条。

- [ ] **步骤 4：提交嵌入产物**

```bash
git add internal/ui/dist
git commit -m "build(web): 重建嵌入前端（P0 设计令牌/基础组件/主题/移动骨架）"
```

---

## P0 完成定义（DoD）

- 明/暗双主题令牌就位并实际驱动外壳与基础组件；主题默认跟随系统、可切换、持久化、首屏不闪烁。
- `style.css` 已拆分入口且现有页面在令牌等价替换下视觉无回归。
- Button/Card/Badge/Spinner/ThemeToggle 基础组件可用、有测试；仅新增 `lucide-react` 一个运行时依赖。
- ≤768px 聊天与设置导航为抽屉 + 遮罩，桌面布局不变。
- 不改任何路由、API 契约、权限；`tsc`/`vitest`/`vite build`/`go build` 全绿。
- 每个任务独立提交，提交信息见各任务。

## 自检结论（计划作者已核对）

- **规格覆盖（P0 范围）**：规格 §3.1 令牌→任务 1；§3.2 图标→任务 4；§3.3 暗色→任务 2/4；§3.4 响应式断点与抽屉→任务 1/5；§7 中 P0 所需的 Button/Card/Badge/Spinner→任务 3（其余组件按"脚手架折进交付物"推迟到 P1/P2）；§8 工程结构/构建嵌入→任务 1/6。规格 §5/§6 的聊天与设置人话化属 P1/P2，不在本计划。
- **类型/命名一致性**：`ThemeMode/ThemeChoice`、`THEME_KEY`、`resolveTheme/systemTheme/applyTheme/readStoredTheme` 在任务 2 定义、任务 4 消费，签名一致；组件 `data-testid` 与测试断言一致（ui-button/ui-badge/ui-card/ui-spinner/ui-theme-toggle）；CSS 类 `app-with-drawer/drawer-open/app-drawer-scrim/app-menu-btn/app-mobile-bar` 在 layout.css 与两个外壳中一致。
- **占位符**：无 TODO/待定；每个代码步骤均给出可运行代码与确切命令。

