import { describe, expect, it } from 'vitest'
import type { Event } from './api'
import { foldToolBlocks, isFirstAssistantMessageOfRun } from './historyBlocks'

function msg(role: string, runId?: string | null) {
  return { role, run_id: runId ?? null }
}

function ev(type: string, data: Record<string, unknown>): Event {
  return { type, timestamp: '', data } as Event
}

describe('foldToolBlocks', () => {
  it('keeps tool/workflow blocks in event order and drops assistant text', () => {
    const events: Event[] = [
      ev('llm.tool_call', { name: 'a' }),
      ev('tool.result', { name: 'a', content: 'ok' }),
      ev('llm.message', { content: 'assistant words' }),
      ev('workflow.started', { skill: 's', steps: ['x'] }),
    ]
    const blocks = foldToolBlocks('run-1', events)
    expect(blocks.map((b) => b.kind)).toEqual(['tool', 'workflow'])
    const tool = blocks[0]
    expect(tool.kind === 'tool' && tool.name).toBe('a')
    expect(tool.kind === 'tool' && tool.status).toBe('succeeded')
    expect(blocks.every((b) => b.runId === 'run-1')).toBe(true)
  })
  it('returns [] when there are no tool/workflow events', () => {
    expect(foldToolBlocks('r', [ev('llm.message', { content: 'hi' })])).toEqual([])
  })
  it('folds HITL waiting into a tool block for read-only rendering', () => {
    const blocks = foldToolBlocks('r', [
      ev('llm.tool_call', { name: 'a' }),
      ev('hitl.waiting', { tool_name: 'a', arguments: { k: 1 } }),
    ])
    expect(blocks).toHaveLength(1)
    expect(blocks[0].kind === 'tool' && blocks[0].status).toBe('waiting_human')
  })
})

describe('isFirstAssistantMessageOfRun', () => {
  it('anchors on the assistant row even when the user row of the same run comes first', () => {
    // Backend persists user(run_id=R) first, then assistant(run_id=R).
    const msgs = [msg('user', 'R'), msg('assistant', 'R')]
    expect(isFirstAssistantMessageOfRun(0, msgs)).toBe(false)
    expect(isFirstAssistantMessageOfRun(1, msgs)).toBe(true)
  })
  it('is true only for the first assistant row when a run has multiple assistant rows', () => {
    const msgs = [
      msg('user', 'R'),
      msg('assistant', 'R'),
      msg('assistant', 'R'),
    ]
    expect(isFirstAssistantMessageOfRun(1, msgs)).toBe(true)
    expect(isFirstAssistantMessageOfRun(2, msgs)).toBe(false)
  })
  it('is false for assistant messages without run_id', () => {
    const msgs = [msg('user', 'R'), msg('assistant', null)]
    expect(isFirstAssistantMessageOfRun(1, msgs)).toBe(false)
  })
  it('anchors the first assistant of each run independently', () => {
    const msgs = [
      msg('user', 'R1'),
      msg('assistant', 'R1'),
      msg('user', 'R2'),
      msg('assistant', 'R2'),
    ]
    expect(isFirstAssistantMessageOfRun(1, msgs)).toBe(true)
    expect(isFirstAssistantMessageOfRun(3, msgs)).toBe(true)
  })
  it('is false for user rows even when they are the first row of the run', () => {
    const msgs = [msg('user', 'R'), msg('assistant', 'R')]
    expect(isFirstAssistantMessageOfRun(0, msgs)).toBe(false)
  })
})
