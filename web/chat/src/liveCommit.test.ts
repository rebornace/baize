import { describe, expect, it } from 'vitest'
import type { ChatMessage, Event } from './api'
import {
  adoptServerMessages,
  commitLiveAssistant,
  handoffTranscript,
  lastLiveAssistantText,
  liveAssistantKey,
  messageRowKey,
} from './liveCommit'

function msg(partial: Partial<ChatMessage> & Pick<ChatMessage, 'id' | 'role'>): ChatMessage {
  return {
    conversation_id: 'c1',
    content: '',
    created_at: 't',
    ...partial,
  }
}

function ev(type: string, data: Record<string, unknown> = {}): Event {
  return { type, timestamp: 't', data }
}

describe('liveCommit', () => {
  it('takes the last live assistant snapshot', () => {
    expect(
      lastLiveAssistantText('run1', [
        ev('llm.content.delta', { turn: 0, text: '先查' }),
        ev('llm.tool_call', { name: 'search' }),
        ev('llm.content.delta', { turn: 1, text: '答案' }),
      ]),
    ).toBe('答案')
  })

  it('appends a local assistant so clearing the live fold does not leave only the user turn', () => {
    const prev = [msg({ id: 'local_u', role: 'user', content: '你好', run_id: 'run1' })]
    const next = commitLiveAssistant(prev, {
      conversationId: 'c1',
      runId: 'run1',
      events: [ev('llm.content.delta', { text: '你好世界' })],
    })
    expect(next.map((m) => m.role)).toEqual(['user', 'assistant'])
    expect(next[1]).toMatchObject({
      id: 'local_asst_run1',
      content: '你好世界',
      run_id: 'run1',
    })
  })

  it('keeps the live snapshot when server messages have not persisted the assistant yet', () => {
    const prev = [
      msg({ id: 'local_u', role: 'user', content: '你好', run_id: 'run1' }),
      msg({ id: 'local_asst_run1', role: 'assistant', content: '答', run_id: 'run1' }),
    ]
    const server = [msg({ id: 'u1', role: 'user', content: '你好', run_id: 'run1' })]
    const next = adoptServerMessages(prev, server, 'run1')
    expect(next.map((m) => m.role)).toEqual(['user', 'assistant'])
    expect(next[1].content).toBe('答')
  })

  it('uses server messages once the assistant is persisted', () => {
    const prev = [
      msg({ id: 'local_asst_run1', role: 'assistant', content: '答', run_id: 'run1' }),
    ]
    const server = [
      msg({ id: 'u1', role: 'user', content: '问', run_id: 'run1' }),
      msg({ id: 'a1', role: 'assistant', content: '答', run_id: 'run1' }),
    ]
    expect(adoptServerMessages(prev, server, 'run1')).toEqual(server)
  })

  it('handoff keeps the live answer when the server has only the user turn', () => {
    const prev = [msg({ id: 'local_u', role: 'user', content: '问', run_id: 'run1' })]
    const server = [msg({ id: 'u1', role: 'user', content: '问', run_id: 'run1' })]
    const next = handoffTranscript(prev, server, {
      conversationId: 'c1',
      runId: 'run1',
      events: [ev('llm.content.delta', { text: '答' })],
    })
    expect(next.map((m) => ({ role: m.role, content: m.content }))).toEqual([
      { role: 'user', content: '问' },
      { role: 'assistant', content: '答' },
    ])
  })

  it('keeps user/assistant row keys stable across local → server ids', () => {
    const local = [
      msg({ id: 'local_u', role: 'user', content: '问', run_id: 'run1' }),
      msg({ id: 'local_asst_run1', role: 'assistant', content: '答', run_id: 'run1' }),
    ]
    const server = [
      msg({ id: 'u1', role: 'user', content: '问', run_id: 'run1' }),
      msg({ id: 'a1', role: 'assistant', content: '答', run_id: 'run1' }),
    ]
    expect(messageRowKey(local[0], 0, local)).toBe(messageRowKey(server[0], 0, server))
    expect(messageRowKey(local[1], 1, local)).toBe(messageRowKey(server[1], 1, server))
  })

  it('uses the same assistant key for the live fold and the persisted row', () => {
    const live = [{ kind: 'thinking' }, { kind: 'assistant' }]
    const persisted = [
      msg({ id: 'u1', role: 'user', content: '问', run_id: 'run1' }),
      msg({ id: 'a1', role: 'assistant', content: '答', run_id: 'run1' }),
    ]
    expect(liveAssistantKey('run1', live, 1)).toBe(messageRowKey(persisted[1], 1, persisted))
  })
})
