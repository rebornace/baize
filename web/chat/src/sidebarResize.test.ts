import { describe, expect, it } from 'vitest'
import {
  applySidebarWidth,
  clampSidebarWidth,
  DEFAULT_SIDEBAR_WIDTH,
  initSidebarWidth,
  MAX_SIDEBAR_WIDTH,
  MIN_SIDEBAR_WIDTH,
  persistSidebarWidth,
  readSidebarWidth,
  SIDEBAR_WIDTH_KEY,
} from './sidebarResize'

function memStorage(initial: Record<string, string> = {}): Storage {
  const map = new Map(Object.entries(initial))
  return {
    getItem: (k: string) => (map.has(k) ? map.get(k)! : null),
    setItem: (k: string, v: string) => void map.set(k, v),
    removeItem: (k: string) => void map.delete(k),
    clear: () => map.clear(),
    key: () => null,
    get length() { return map.size },
  } as unknown as Storage
}

function fakeRoot(): HTMLElement & { last: Record<string, string> } {
  const set: Record<string, string> = {}
  const el = { style: { setProperty: (k: string, v: string) => { set[k] = v } } }
  return Object.assign(el as unknown as HTMLElement, { get last() { return set } })
}

describe('clampSidebarWidth', () => {
  it('keeps in-range values (rounded)', () => {
    expect(clampSidebarWidth(260)).toBe(260)
    expect(clampSidebarWidth(300.4)).toBe(300)
  })
  it('clamps below min and above max', () => {
    expect(clampSidebarWidth(50)).toBe(MIN_SIDEBAR_WIDTH)
    expect(clampSidebarWidth(5000)).toBe(MAX_SIDEBAR_WIDTH)
  })
  it('falls back to default for non-finite input', () => {
    expect(clampSidebarWidth(NaN)).toBe(DEFAULT_SIDEBAR_WIDTH)
    expect(clampSidebarWidth(Infinity)).toBe(DEFAULT_SIDEBAR_WIDTH)
  })
})

describe('readSidebarWidth', () => {
  it('returns null when storage unavailable or key missing', () => {
    expect(readSidebarWidth(null)).toBeNull()
    expect(readSidebarWidth(memStorage())).toBeNull()
  })
  it('parses and clamps a valid saved width', () => {
    expect(readSidebarWidth(memStorage({ [SIDEBAR_WIDTH_KEY]: '300' }))).toBe(300)
    expect(readSidebarWidth(memStorage({ [SIDEBAR_WIDTH_KEY]: '9000' }))).toBe(MAX_SIDEBAR_WIDTH)
  })
  it('returns null for non-numeric saved value', () => {
    expect(readSidebarWidth(memStorage({ [SIDEBAR_WIDTH_KEY]: 'abc' }))).toBeNull()
  })
})

describe('persistSidebarWidth', () => {
  it('clamps and stores the value', () => {
    const s = memStorage()
    expect(persistSidebarWidth(120, s)).toBe(MIN_SIDEBAR_WIDTH)
    expect(s.getItem(SIDEBAR_WIDTH_KEY)).toBe(String(MIN_SIDEBAR_WIDTH))
  })
  it('does not throw without storage', () => {
    expect(() => persistSidebarWidth(260, null)).not.toThrow()
  })
})

describe('applySidebarWidth', () => {
  it('sets --sidebar-width in px and returns clamped value', () => {
    const root = fakeRoot()
    expect(applySidebarWidth(310, root)).toBe(310)
    expect(root.last['--sidebar-width']).toBe('310px')
    applySidebarWidth(10, root)
    expect(root.last['--sidebar-width']).toBe(`${MIN_SIDEBAR_WIDTH}px`)
  })
})

describe('initSidebarWidth', () => {
  it('applies saved width and returns it', () => {
    const root = fakeRoot()
    const got = initSidebarWidth(memStorage({ [SIDEBAR_WIDTH_KEY]: '288' }), root)
    expect(got).toBe(288)
    expect(root.last['--sidebar-width']).toBe('288px')
  })
  it('returns null and writes nothing when no saved width', () => {
    const root = fakeRoot()
    expect(initSidebarWidth(memStorage(), root)).toBeNull()
    expect(root.last['--sidebar-width']).toBeUndefined()
  })
})
