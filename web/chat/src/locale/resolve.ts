import { readStoredLocale } from './storage'
import type { Locale } from './types'

export type BrowserLanguageSource = {
  language?: string
  languages?: readonly string[]
}

/** Map BCP-47 tags: zh* → zh-CN, everything else → en. */
export function localeFromBrowser(nav: BrowserLanguageSource = typeof navigator !== 'undefined' ? navigator : {}): Locale {
  const tags = [
    ...(nav.languages ?? []),
    ...(nav.language ? [nav.language] : []),
  ]
  for (const tag of tags) {
    if (!tag) continue
    if (tag.toLowerCase().startsWith('zh')) return 'zh-CN'
  }
  return 'en'
}

/**
 * Preference order: valid localStorage → browser → en.
 */
export function resolveLocale(opts?: {
  storage?: Pick<Storage, 'getItem'>
  navigator?: BrowserLanguageSource
}): Locale {
  const stored = readStoredLocale(opts?.storage ?? (typeof localStorage !== 'undefined' ? localStorage : { getItem: () => null }))
  if (stored) return stored
  return localeFromBrowser(opts?.navigator ?? (typeof navigator !== 'undefined' ? navigator : {}))
}
