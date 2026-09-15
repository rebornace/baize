import { describe, expect, it } from 'vitest'
import { localeFromBrowser, resolveLocale } from './resolve'
import { readStoredLocale, writeStoredLocale } from './storage'
import { LOCALE_STORAGE_KEY } from './types'

function memStorage(initial: Record<string, string> = {}): Storage {
  const map = new Map(Object.entries(initial))
  return {
    get length() {
      return map.size
    },
    clear: () => map.clear(),
    getItem: (k) => (map.has(k) ? map.get(k)! : null),
    setItem: (k, v) => {
      map.set(k, String(v))
    },
    removeItem: (k) => {
      map.delete(k)
    },
    key: (i) => [...map.keys()][i] ?? null,
  }
}

describe('localeFromBrowser', () => {
  it('maps zh* to zh-CN', () => {
    expect(localeFromBrowser({ language: 'zh-CN' })).toBe('zh-CN')
    expect(localeFromBrowser({ language: 'zh-TW' })).toBe('zh-CN')
    expect(localeFromBrowser({ languages: ['zh'] })).toBe('zh-CN')
  })

  it('maps non-zh to en', () => {
    expect(localeFromBrowser({ language: 'en-US' })).toBe('en')
    expect(localeFromBrowser({ language: 'ja' })).toBe('en')
    expect(localeFromBrowser({})).toBe('en')
  })
})

describe('resolveLocale', () => {
  it('prefers valid storage over browser', () => {
    const storage = memStorage({ [LOCALE_STORAGE_KEY]: 'en' })
    expect(resolveLocale({ storage, navigator: { language: 'zh-CN' } })).toBe('en')
  })

  it('uses browser when storage empty', () => {
    const storage = memStorage()
    expect(resolveLocale({ storage, navigator: { language: 'zh-CN' } })).toBe('zh-CN')
    expect(resolveLocale({ storage, navigator: { language: 'en-US' } })).toBe('en')
  })

  it('ignores invalid storage', () => {
    const storage = memStorage({ [LOCALE_STORAGE_KEY]: 'fr' })
    expect(resolveLocale({ storage, navigator: { language: 'zh' } })).toBe('zh-CN')
  })
})

describe('storage', () => {
  it('round-trips locale', () => {
    const storage = memStorage()
    expect(readStoredLocale(storage)).toBeNull()
    writeStoredLocale('en', storage)
    expect(readStoredLocale(storage)).toBe('en')
  })
})
