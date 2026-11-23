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

/**
 * 解析最终明暗。
 * - light/dark：按存储
 * - system：跟随 system()
 * - 空/未知：默认 light（宣传版首屏；用户仍可选 system）
 */
export function resolveTheme(stored: string, system: () => ThemeMode): ThemeMode {
  if (stored === 'light' || stored === 'dark') return stored
  if (stored === 'system') return system()
  return 'light'
}

/** 把最终明暗写到 <html data-theme>。抽出 root 参数便于在 node 环境测试。 */
export function applyTheme(mode: ThemeMode, root: HTMLElement): void {
  root.setAttribute('data-theme', mode)
}
