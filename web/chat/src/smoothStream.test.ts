import { describe, expect, it } from 'vitest'
import { advanceByGraphemes, drainRate, MAX_CPS, MIN_CPS } from './smoothStream'

describe('drainRate', () => {
  it('clamps to MIN/MAX and scales with backlog', () => {
    expect(drainRate(0)).toBe(MIN_CPS)
    expect(drainRate(1)).toBeGreaterThanOrEqual(MIN_CPS)
    expect(drainRate(100_000)).toBe(MAX_CPS)
    expect(drainRate(200)).toBeGreaterThan(drainRate(20))
  })
})

describe('advanceByGraphemes', () => {
  it('advances by code points / graphemes without splitting surrogates', () => {
    const text = '你好😀世界'
    const once = advanceByGraphemes(text, 0, 1)
    expect(text.slice(0, once)).toBe('你')
    const two = advanceByGraphemes(text, 0, 2)
    expect(text.slice(0, two)).toBe('你好')
    const three = advanceByGraphemes(text, 0, 3)
    expect(text.slice(0, three)).toBe('你好😀')
  })

  it('no-ops when budget is 0', () => {
    expect(advanceByGraphemes('abc', 1, 0)).toBe(1)
  })
})
