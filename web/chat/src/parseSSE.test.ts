import { describe, expect, it } from 'vitest'
import { consumeSSE } from './parseSSE'

const SAMPLE = `id: 1
data: {"type":"llm.message","timestamp":"t","data":{"content":"hi"}}

event: run.ended
data: {"status":"succeeded"}

`

describe('consumeSSE', () => {
  it('parses a default llm.message frame and run.ended', () => {
    const { frames, rest } = consumeSSE(SAMPLE)
    expect(rest).toBe('')
    expect(frames).toHaveLength(2)

    expect(frames[0].id).toBe('1')
    expect(frames[0].event).toBe('')
    expect(frames[0].data).toContain('llm.message')

    expect(frames[1].event).toBe('run.ended')
    expect(frames[1].data).toContain('succeeded')
  })
})
