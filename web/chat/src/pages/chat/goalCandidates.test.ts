import { describe, expect, it } from 'vitest'
import { goalCandidatesFromMessages } from './goalCandidates'

describe('goalCandidatesFromMessages', () => {
  it('returns recent unique user texts newest first', () => {
    const out = goalCandidatesFromMessages([
      { id: '1', role: 'user', content: '先做 A' },
      { id: '2', role: 'assistant', content: '好的' },
      { id: '3', role: 'user', content: '再做 B' },
      { id: '4', role: 'user', content: '先做 A' },
    ])
    expect(out.map((c) => c.text)).toEqual(['先做 A', '再做 B'])
    expect(out[0]?.id).toBe('4')
  })

  it('strips channel media and skips empty', () => {
    const out = goalCandidatesFromMessages([
      {
        id: '1',
        role: 'user',
        content: '![图片](/v0/channels/media/c1/a.jpg)',
      },
      {
        id: '2',
        role: 'user',
        content: '看看这个图 ![图片](/v0/channels/media/c1/b.jpg) 然后修 bug',
      },
    ])
    expect(out).toHaveLength(1)
    expect(out[0]?.text).toContain('修 bug')
    expect(out[0]?.text).not.toContain('![图片]')
  })

  it('caps list length', () => {
    const messages = Array.from({ length: 30 }, (_, i) => ({
      id: String(i),
      role: 'user',
      content: `msg ${i}`,
    }))
    expect(goalCandidatesFromMessages(messages, 5)).toHaveLength(5)
  })
})
