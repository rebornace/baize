import { describe, expect, it } from 'vitest'
import { waitedLabel } from './waitTime'

const units = {
  waitedMinutes: (n: number) => `waiting ${n} min`,
  waitedHours: (n: number) => `waiting ${n} h`,
  waitedDays: (n: number) => `waiting ${n} d`,
}

const now = Date.parse('2026-11-19T12:00:00Z')

describe('waitedLabel', () => {
  it('formats minutes, hours and days', () => {
    expect(waitedLabel(new Date(now - 5 * 60_000).toISOString(), now, units)).toBe('waiting 5 min')
    expect(waitedLabel(new Date(now - 3 * 3_600_000).toISOString(), now, units)).toBe('waiting 3 h')
    expect(waitedLabel(new Date(now - 2 * 86_400_000).toISOString(), now, units)).toBe('waiting 2 d')
  })

  it('rounds under a minute up to one minute', () => {
    expect(waitedLabel(new Date(now - 10_000).toISOString(), now, units)).toBe('waiting 1 min')
  })

  it('returns null without a valid timestamp', () => {
    expect(waitedLabel(undefined, now, units)).toBeNull()
    expect(waitedLabel('not-a-date', now, units)).toBeNull()
  })
})
