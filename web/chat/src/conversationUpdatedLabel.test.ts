import { describe, expect, it, vi, afterEach } from 'vitest'
import { conversationUpdatedLabel } from './conversationUpdatedLabel'

describe('conversationUpdatedLabel', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns empty for invalid iso', () => {
    expect(conversationUpdatedLabel('not-a-date', 'zh-CN')).toBe('')
  })

  it('formats same-day as time in zh-CN', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-22T14:30:00Z'))
    const label = conversationUpdatedLabel('2026-10-22T10:15:00Z', 'zh-CN')
    expect(label.length).toBeGreaterThan(0)
    expect(label).not.toContain('2026')
  })

  it('formats prior year with year in en', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-22T12:00:00Z'))
    const label = conversationUpdatedLabel('2025-03-01T08:00:00Z', 'en')
    expect(label).toMatch(/2025/)
  })
})
