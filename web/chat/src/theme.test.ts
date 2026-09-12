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
