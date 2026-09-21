import { describe, expect, it } from 'vitest'
import type { Event } from './api'
import { foldEvents } from './foldEvents'

function ev(type: string, data?: Record<string, unknown>): Event {
  return { type, timestamp: '2026-08-15T00:00:00Z', data }
}

describe('foldEvents', () => {
  it('folds tool_call + tool.result into one card', () => {
    const events: Event[] = [
      ev('llm.tool_call', { name: 'create_ticket', arguments: { title: 'VPN' } }),
      ev('tool.result', { name: 'create_ticket', content: { id: '1' }, is_error: false }),
    ]
    const blocks = foldEvents('run_1', events)
    expect(blocks).toEqual([
      {
        kind: 'tool',
        name: 'create_ticket',
        status: 'succeeded',
        arguments: { title: 'VPN' },
        result: { id: '1' },
        isError: false,
        runId: 'run_1',
      },
    ])
  })

  it('marks waiting_human so ToolCard can show approve/reject', () => {
    const events: Event[] = [
      ev('llm.tool_call', { name: 'create_ticket', arguments: { title: 'VPN' } }),
      ev('hitl.waiting', {
        tool_name: 'create_ticket',
        prompt: 'Approve tool create_ticket?',
        arguments: { title: 'VPN' },
      }),
    ]
    const blocks = foldEvents('run_2', events)
    expect(blocks).toHaveLength(1)
    expect(blocks[0]).toMatchObject({
      kind: 'tool',
      name: 'create_ticket',
      status: 'waiting_human',
      runId: 'run_2',
    })
  })

  it('maps hitl.resumed / rejected and llm.message / llm.error', () => {
    const events: Event[] = [
      ev('llm.tool_call', { name: 'create_ticket', arguments: {} }),
      ev('hitl.waiting', { tool_name: 'create_ticket', prompt: 'ok' }),
      ev('hitl.resumed', { decision: 'approve', comment: 'lgtm' }),
      ev('tool.result', { name: 'create_ticket', content: { id: '9' }, is_error: false }),
      ev('llm.message', { content: '已创建工单' }),
      ev('llm.error', { error: 'boom' }),
    ]
    const blocks = foldEvents('run_3', events)
    expect(blocks[0]).toMatchObject({
      kind: 'tool',
      name: 'create_ticket',
      status: 'succeeded',
      result: { id: '9' },
    })
    expect(blocks[1]).toEqual({ kind: 'assistant', text: '已创建工单' })
    expect(blocks[2]).toEqual({ kind: 'system', text: 'boom' })
  })

  it('maps hitl.rejected to rejected status on the same card', () => {
    const events: Event[] = [
      ev('llm.tool_call', { name: 'create_ticket', arguments: {} }),
      ev('hitl.waiting', { tool_name: 'create_ticket' }),
      ev('hitl.rejected', { decision: 'reject', comment: 'no' }),
    ]
    const blocks = foldEvents('run_4', events)
    expect(blocks).toEqual([
      {
        kind: 'tool',
        name: 'create_ticket',
        status: 'rejected',
        arguments: {},
        runId: 'run_4',
      },
    ])
  })

  it('maps hitl.resumed to approved without following tool.result', () => {
    const events: Event[] = [
      ev('llm.tool_call', { name: 'create_ticket', arguments: {} }),
      ev('hitl.waiting', { tool_name: 'create_ticket', prompt: 'ok' }),
      ev('hitl.resumed', { decision: 'approve', comment: 'lgtm' }),
    ]
    const blocks = foldEvents('run_approved', events)
    expect(blocks).toEqual([
      {
        kind: 'tool',
        name: 'create_ticket',
        status: 'approved',
        arguments: {},
        runId: 'run_approved',
      },
    ])
  })

  it('rejects only the named waiting card when two tools wait in parallel', () => {
    const events: Event[] = [
      ev('llm.tool_call', { name: 'tool_a', arguments: { x: 1 } }),
      ev('hitl.waiting', { tool_name: 'tool_a', arguments: { x: 1 } }),
      ev('llm.tool_call', { name: 'tool_b', arguments: { y: 2 } }),
      ev('hitl.waiting', { tool_name: 'tool_b', arguments: { y: 2 } }),
      ev('hitl.rejected', { tool_name: 'tool_a', decision: 'reject', comment: 'no' }),
    ]
    const blocks = foldEvents('run_parallel', events)
    expect(blocks).toEqual([
      {
        kind: 'tool',
        name: 'tool_a',
        status: 'rejected',
        arguments: { x: 1 },
        runId: 'run_parallel',
      },
      {
        kind: 'tool',
        name: 'tool_b',
        status: 'waiting_human',
        arguments: { y: 2 },
        runId: 'run_parallel',
      },
    ])
  })

  it('folds workflow events into a progress block', () => {
    let blocks = foldEvents('run_wf', [
      ev('workflow.started', { skill: 'triage', steps: ['fetch', 'reply'] }),
    ])
    expect(blocks.at(-1)).toEqual({
      kind: 'workflow',
      skill: 'triage',
      steps: [
        { id: 'fetch', status: 'pending' },
        { id: 'reply', status: 'pending' },
      ],
      runId: 'run_wf',
    })

    blocks = foldEvents('run_wf', [
      ev('workflow.started', { skill: 'triage', steps: ['fetch', 'reply'] }),
      ev('workflow.step_started', { step: 'fetch', tool: 'search_tickets' }),
      ev('workflow.step_completed', { step: 'fetch', is_error: false }),
      ev('workflow.step_started', { step: 'reply', tool: 'reply_ticket' }),
    ])
    const wf = blocks.at(-1)
    expect(wf?.kind).toBe('workflow')
    if (wf?.kind === 'workflow') {
      expect(wf.steps[0].status).toBe('done')
      expect(wf.steps[1].status).toBe('running')
    }
  })

  it('marks workflow step failed when step_completed has is_error', () => {
    const blocks = foldEvents('run_wf_fail', [
      ev('workflow.started', { skill: 'triage', steps: ['fetch'] }),
      ev('workflow.step_started', { step: 'fetch', tool: 'search_tickets' }),
      ev('workflow.step_completed', { step: 'fetch', is_error: true }),
    ])
    const wf = blocks.at(-1)
    expect(wf?.kind).toBe('workflow')
    if (wf?.kind === 'workflow') {
      expect(wf.steps[0].status).toBe('failed')
    }
  })

  it('matches tool.result to the topmost unfinished same-name tool', () => {
    const events: Event[] = [
      ev('llm.tool_call', { name: 'echo', arguments: { n: 1 } }),
      ev('tool.result', { name: 'echo', content: 'a', is_error: false }),
      ev('llm.tool_call', { name: 'echo', arguments: { n: 2 } }),
      ev('tool.result', { name: 'echo', content: 'b', is_error: true }),
    ]
    const blocks = foldEvents('run_5', events)
    expect(blocks).toEqual([
      {
        kind: 'tool',
        name: 'echo',
        status: 'succeeded',
        arguments: { n: 1 },
        result: 'a',
        isError: false,
        runId: 'run_5',
      },
      {
        kind: 'tool',
        name: 'echo',
        status: 'failed',
        arguments: { n: 2 },
        result: 'b',
        isError: true,
        runId: 'run_5',
      },
    ])
  })

  it('folds thinking.delta into a streaming uncollapsed block', () => {
    const blocks = foldEvents('run_think', [
      ev('llm.thinking.delta', { turn: 0, text: '先想一步' }),
    ])
    expect(blocks).toEqual([
      {
        kind: 'thinking',
        turn: 0,
        text: '先想一步',
        status: 'streaming',
        collapsed: false,
      },
    ])
  })

  it('collapses thinking and streams assistant text on content.delta', () => {
    const blocks = foldEvents('run_think', [
      ev('llm.thinking.delta', { turn: 0, text: '推理中' }),
      ev('llm.thinking.delta', { turn: 0, text: '推理中…完成' }),
      ev('llm.content.delta', { turn: 0, text: '你好' }),
      ev('llm.content.delta', { turn: 0, text: '你好，世界' }),
    ])
    expect(blocks).toEqual([
      {
        kind: 'thinking',
        turn: 0,
        text: '推理中…完成',
        status: 'done',
        collapsed: true,
      },
      { kind: 'assistant', text: '你好，世界' },
    ])
  })

  it('marks llm.thinking redacted without creating an assistant', () => {
    const blocks = foldEvents('run_redact', [
      ev('llm.thinking', { turn: 0, text: '', thinking_redacted: true }),
      ev('llm.message', { content: '最终回答', thinking: '' }),
    ])
    expect(blocks).toEqual([
      {
        kind: 'thinking',
        turn: 0,
        text: '',
        status: 'redacted',
        collapsed: true,
      },
      { kind: 'assistant', text: '最终回答' },
    ])
  })

  it('does not emit an extra assistant from thinking on llm.message', () => {
    const blocks = foldEvents('run_msg', [
      ev('llm.thinking.delta', { turn: 0, text: '内部思考' }),
      ev('llm.thinking', { turn: 0, text: '内部思考', thinking_redacted: false }),
      ev('llm.message', { content: '对外回答', thinking: '内部思考' }),
    ])
    expect(blocks.filter((b) => b.kind === 'assistant')).toEqual([
      { kind: 'assistant', text: '对外回答' },
    ])
    expect(blocks.filter((b) => b.kind === 'thinking')).toHaveLength(1)
  })

  it('keeps per-turn thinking blocks before tool cards', () => {
    const blocks = foldEvents('run_multi', [
      ev('llm.thinking.delta', { turn: 0, text: 'T0' }),
      ev('llm.thinking', { turn: 0, text: 'T0' }),
      ev('llm.tool_call', { name: 'search', arguments: { q: 'x' } }),
      ev('tool.result', { name: 'search', content: 'hit', is_error: false }),
      ev('llm.thinking.delta', { turn: 1, text: 'T1' }),
      ev('llm.content.delta', { turn: 1, text: '结论' }),
    ])
    expect(blocks.map((b) => b.kind)).toEqual([
      'thinking',
      'tool',
      'thinking',
      'assistant',
    ])
    expect(blocks[0]).toMatchObject({ kind: 'thinking', turn: 0, text: 'T0', status: 'done' })
    expect(blocks[2]).toMatchObject({
      kind: 'thinking',
      turn: 1,
      text: 'T1',
      status: 'done',
      collapsed: true,
    })
    expect(blocks[3]).toEqual({ kind: 'assistant', text: '结论' })
  })

  it('aggregates run usage onto the terminal assistant block', () => {
    const blocks = foldEvents('run_usage', [
      ev('llm.usage', { turn: 0, prompt_tokens: 10, completion_tokens: 4, total_tokens: 14 }),
      ev('llm.message', {
        content: '已创建',
        turn: 1,
        prompt_tokens: 20,
        completion_tokens: 6,
        total_tokens: 26,
      }),
    ])
    expect(blocks).toHaveLength(1)
    expect(blocks[0]).toMatchObject({
      kind: 'assistant',
      text: '已创建',
      usage: { promptTokens: 30, completionTokens: 10, totalTokens: 40, savedTokens: 0 },
    })
  })

  it('records memory-extraction savings onto the terminal assistant block', () => {
    const blocks = foldEvents('run_saved', [
      ev('memory.extract_skipped', { reason: 'decider_no', source: 'rules', saved_tokens: 53 }),
      ev('llm.message', { content: '你好呀' }),
    ])
    expect(blocks).toHaveLength(1)
    expect(blocks[0]).toMatchObject({
      kind: 'assistant',
      text: '你好呀',
      usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0, savedTokens: 53 },
    })
  })

  it('does not attach usage when nothing reported tokens or savings', () => {
    const blocks = foldEvents('run_plain', [
      ev('llm.message', { content: '无用量' }),
    ])
    expect(blocks).toEqual([{ kind: 'assistant', text: '无用量' }])
  })
})
