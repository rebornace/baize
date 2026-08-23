import { describe, expect, it } from 'vitest'
import type { ChatMessage } from './api'
import { findLiveRunCandidate, isActiveRunStatus } from './findLiveRun'

function msg(
  partial: Pick<ChatMessage, 'role' | 'content'> & Partial<ChatMessage>,
): ChatMessage {
  return {
    id: partial.id ?? `m_${Math.random().toString(36).slice(2)}`,
    conversation_id: partial.conversation_id ?? 'conv_1',
    role: partial.role,
    content: partial.content,
    run_id: partial.run_id,
    created_at: partial.created_at ?? '2026-08-15T00:00:00Z',
  }
}

describe('findLiveRunCandidate', () => {
  it('returns null when there are no messages', () => {
    expect(findLiveRunCandidate([])).toBeNull()
  })

  it('returns null when every run_id has an assistant message', () => {
    const messages = [
      msg({ role: 'user', content: 'hi', run_id: 'run_1' }),
      msg({ role: 'assistant', content: 'done', run_id: 'run_1' }),
    ]
    expect(findLiveRunCandidate(messages)).toBeNull()
  })

  it('returns run_id of a waiting_human turn (user only, no assistant)', () => {
    const messages = [
      msg({ role: 'user', content: 'older', run_id: 'run_0' }),
      msg({ role: 'assistant', content: 'ok', run_id: 'run_0' }),
      msg({ role: 'user', content: 'please ticket', run_id: 'run_hitl' }),
    ]
    expect(findLiveRunCandidate(messages)).toBe('run_hitl')
  })

  it('prefers the newest unsettled run_id', () => {
    const messages = [
      msg({ role: 'user', content: 'a', run_id: 'run_old' }),
      msg({ role: 'user', content: 'b', run_id: 'run_new' }),
    ]
    expect(findLiveRunCandidate(messages)).toBe('run_new')
  })

  it('treats system_note as settling a run', () => {
    const messages = [
      msg({ role: 'user', content: 'x', run_id: 'run_fail' }),
      msg({ role: 'system_note', content: 'rejected', run_id: 'run_fail' }),
    ]
    expect(findLiveRunCandidate(messages)).toBeNull()
  })
})

describe('isActiveRunStatus', () => {
  it('is true for queued/running/waiting_human', () => {
    expect(isActiveRunStatus('queued')).toBe(true)
    expect(isActiveRunStatus('running')).toBe(true)
    expect(isActiveRunStatus('waiting_human')).toBe(true)
  })

  it('is false for terminal statuses', () => {
    expect(isActiveRunStatus('succeeded')).toBe(false)
    expect(isActiveRunStatus('failed')).toBe(false)
    expect(isActiveRunStatus('cancelled')).toBe(false)
  })
})
