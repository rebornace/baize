// 侧栏宽度拖拽：聊天页与设置页共用同一宽度。纯逻辑可单测；DOM 副作用集中在 apply。

export const SIDEBAR_WIDTH_KEY = 'baize.sidebar_width'
export const MIN_SIDEBAR_WIDTH = 200
export const MAX_SIDEBAR_WIDTH = 420
export const DEFAULT_SIDEBAR_WIDTH = 240

/** 将像素值夹取到允许的侧栏宽度区间，并取整。非有限数回落到默认宽度。 */
export function clampSidebarWidth(px: number): number {
  if (!Number.isFinite(px)) return DEFAULT_SIDEBAR_WIDTH
  return Math.round(Math.min(MAX_SIDEBAR_WIDTH, Math.max(MIN_SIDEBAR_WIDTH, px)))
}

/**
 * 读取已保存的宽度。无值或非法时返回 null（调用方回落默认，不覆盖令牌）。
 * 注入 storage 便于测试。
 */
export function readSidebarWidth(storage: Storage | null = safeLocalStorage()): number | null {
  if (!storage) return null
  const raw = storage.getItem(SIDEBAR_WIDTH_KEY)
  if (raw == null) return null
  const n = Number(raw)
  if (!Number.isFinite(n)) return null
  return clampSidebarWidth(n)
}

/** 夹取并持久化宽度，返回夹取后的像素值。 */
export function persistSidebarWidth(px: number, storage: Storage | null = safeLocalStorage()): number {
  const clamped = clampSidebarWidth(px)
  storage?.setItem(SIDEBAR_WIDTH_KEY, String(clamped))
  return clamped
}

/** 把宽度写入 :root 的 --sidebar-width，驱动两个侧栏。 */
export function applySidebarWidth(px: number, root: HTMLElement = document.documentElement): number {
  const clamped = clampSidebarWidth(px)
  root.style.setProperty('--sidebar-width', `${clamped}px`)
  return clamped
}

/** 启动时调用：用已保存宽度（若有）覆盖默认令牌；无保存则保持 CSS 默认。 */
export function initSidebarWidth(
  storage: Storage | null = safeLocalStorage(),
  root: HTMLElement = document.documentElement,
): number | null {
  const saved = readSidebarWidth(storage)
  if (saved == null) return null
  applySidebarWidth(saved, root)
  return saved
}

function safeLocalStorage(): Storage | null {
  try {
    return window.localStorage
  } catch {
    return null
  }
}
