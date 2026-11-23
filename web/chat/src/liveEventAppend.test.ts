import { describe, expect, it } from 'vitest'
import type { Event } from './api'
import { appendLiveEvent, coalesceLiveEvents } from './liveEventAppend'

const delta = (text: string, turn = 0): Event => ({
  type: 'llm.content.delta',
  timestamp: 't',
  data: { turn, text },
})

describe('appendLiveEvent', () => {
  it('coalesces consecutive content deltas for the same turn', () => {
    let evs: Event[] = []
    evs = appendLiveEvent(evs, delta('你'))
    evs = appendLiveEvent(evs, delta('你好'))
    evs = appendLiveEvent(evs, delta('你好世界'))
    expect(evs).toHaveLength(1)
    expect(evs[0].data?.text).toBe('你好世界')
  })

  it('does not coalesce across turns or other event types', () => {
    let evs: Event[] = []
    evs = appendLiveEvent(evs, delta('a', 0))
    evs = appendLiveEvent(evs, { type: 'tool.result', timestamp: 't', data: { name: 'x' } })
    evs = appendLiveEvent(evs, delta('b', 1))
    expect(evs).toHaveLength(3)
  })
})

describe('coalesceLiveEvents', () => {
  it('collapses a polled history of many deltas', () => {
    const raw = [delta('a'), delta('ab'), delta('abc'), delta('x', 1), delta('xy', 1)]
    const out = coalesceLiveEvents(raw)
    expect(out).toHaveLength(2)
    expect(out[0].data?.text).toBe('abc')
    expect(out[1].data?.text).toBe('xy')
  })
})
