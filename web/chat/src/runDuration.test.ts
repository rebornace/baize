import { describe, expect, it } from 'vitest'
import type { Event } from './api'
import { formatWallDuration, runDurationMsFromEvents } from './runDuration'

function ev(timestamp: string, type = 'llm.message'): Event {
  return { type, timestamp, data: {} }
}

describe('runDurationMsFromEvents', () => {
  it('returns span from earliest to latest timestamp', () => {
    const events = [
      ev('2026-10-24T04:00:00.000Z'),
      ev('2026-10-24T04:00:12.500Z'),
      ev('2026-10-24T04:00:05.000Z'),
    ]
    expect(runDurationMsFromEvents(events)).toBe(12500)
  })

  it('uses untilMs when provided (live clock)', () => {
    const start = Date.parse('2026-10-24T04:00:00.000Z')
    expect(
      runDurationMsFromEvents([ev('2026-10-24T04:00:00.000Z')], { untilMs: start + 3200 }),
    ).toBe(3200)
  })

  it('returns undefined when no parseable timestamps', () => {
    expect(runDurationMsFromEvents([ev('')])).toBeUndefined()
    expect(runDurationMsFromEvents([])).toBeUndefined()
  })
})

describe('formatWallDuration', () => {
  it('formats short seconds in zh and en', () => {
    expect(formatWallDuration(3200, 'zh-CN')).toBe('3.2 秒')
    expect(formatWallDuration(3200, 'en')).toBe('3.2s')
  })

  it('formats minutes', () => {
    expect(formatWallDuration(65_000, 'zh-CN')).toBe('1 分 05 秒')
    expect(formatWallDuration(65_000, 'en')).toBe('1m 05s')
  })
})
