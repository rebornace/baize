import { describe, expect, it } from 'vitest'
import type { Event } from './api'
import { foldToolBlocks } from './historyBlocks'

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
