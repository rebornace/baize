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

/** 存储值合法直接采用；否则（空/未知）回退到系统偏好（由调用方惰性提供）。 */
export function resolveTheme(stored: string, system: () => ThemeMode): ThemeMode {
  if (stored === 'light' || stored === 'dark') return stored
  return system()
}

/** 把最终明暗写到 <html data-theme>。抽出 root 参数便于在 node 环境测试。 */
export function applyTheme(mode: ThemeMode, root: HTMLElement): void {
  root.setAttribute('data-theme', mode)
}
