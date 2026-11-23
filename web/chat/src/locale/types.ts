export type Locale = 'zh-CN' | 'en'

export const LOCALES: readonly Locale[] = ['zh-CN', 'en'] as const

export const LOCALE_STORAGE_KEY = 'baize.locale'

export function isLocale(v: unknown): v is Locale {
  return v === 'zh-CN' || v === 'en'
}
