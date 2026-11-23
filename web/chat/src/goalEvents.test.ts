import { describe, expect, it } from 'vitest'
import { goalEventCount } from './goalEvents'
import type { Event } from './api'

function ev(type: string): Event {
  return { type, timestamp: '2026-11-19T00:00:00Z' }
}

describe('goalEventCount', () => {
  it('counts goal.* events only', () => {
    expect(goalEventCount(undefined)).toBe(0)
    expect(goalEventCount([])).toBe(0)
    expect(goalEventCount([ev('run.started'), ev('llm.message')])).toBe(0)
    expect(goalEventCount([ev('goal.seeded'), ev('llm.message'), ev('goal.pushed')])).toBe(2)
  })
})
