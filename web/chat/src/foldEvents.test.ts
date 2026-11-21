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

  it('surfaces channel.outbound.dead as a system note', () => {
    const blocks = foldEvents('run_dead', [
      ev('channel.outbound.dead', { channel: 'weixin', error: 'webhook: outbound status 400' }),
    ])
    expect(blocks).toEqual([
      { kind: 'system', text: 'weixin 出站失败：webhook: outbound status 400' },
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
    expect(blocks[1]).toEqual({ kind: 'assistant', text: '已创建工单', streaming: false })
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
        waitingSince: '2026-08-15T00:00:00Z',
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
        waitingSince: '2026-08-15T00:00:00Z',
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
        waitingSince: '2026-08-15T00:00:00Z',
      },
      {
        kind: 'tool',
        name: 'tool_b',
        status: 'waiting_human',
        arguments: { y: 2 },
        runId: 'run_parallel',
        waitingSince: '2026-08-15T00:00:00Z',
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
      { kind: 'assistant', text: '你好，世界', streaming: true },
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
      { kind: 'assistant', text: '最终回答', streaming: false },
    ])
  })

  it('does not emit an extra assistant from thinking on llm.message', () => {
    const blocks = foldEvents('run_msg', [
      ev('llm.thinking.delta', { turn: 0, text: '内部思考' }),
      ev('llm.thinking', { turn: 0, text: '内部思考', thinking_redacted: false }),
      ev('llm.message', { content: '对外回答', thinking: '内部思考' }),
    ])
    expect(blocks.filter((b) => b.kind === 'assistant')).toEqual([
      { kind: 'assistant', text: '对外回答', streaming: false },
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
    expect(blocks[3]).toEqual({ kind: 'assistant', text: '结论', streaming: true })
  })

  it('keeps first-turn assistant text above tools and streams later text after them', () => {
    const blocks = foldEvents('run_preamble', [
      ev('llm.content.delta', { turn: 0, text: '我先查一下' }),
      ev('llm.tool_call', { name: 'search', arguments: { q: 'x' } }),
      ev('tool.result', { name: 'search', content: 'hit', is_error: false }),
      ev('llm.content.delta', { turn: 1, text: '找到了' }),
    ])
    expect(blocks.map((b) => b.kind)).toEqual(['assistant', 'tool', 'assistant'])
    expect(blocks[0]).toEqual({ kind: 'assistant', text: '我先查一下', streaming: false })
    expect(blocks[2]).toEqual({ kind: 'assistant', text: '找到了', streaming: true })
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

  it('aggregates cache-hit tokens across turns', () => {
    const blocks = foldEvents('run_cached', [
      ev('llm.usage', { turn: 0, prompt_tokens: 10, completion_tokens: 4, total_tokens: 14, cached_tokens: 8 }),
      ev('llm.message', {
        content: '已创建',
        turn: 1,
        prompt_tokens: 20,
        completion_tokens: 6,
        total_tokens: 26,
        cached_tokens: 15,
      }),
    ])
    expect(blocks).toHaveLength(1)
    expect(blocks[0]).toMatchObject({
      kind: 'assistant',
      text: '已创建',
      usage: { promptTokens: 30, totalTokens: 40, cachedTokens: 23 },
    })
  })

  it('marks the waiting workflow step from structured HITL data', () => {
    const events: Event[] = [
      ev('workflow.started', { skill: 'demo', steps: ['a', 'b'] }),
      ev('workflow.step_started', { step: 'a', tool: 'ta' }),
      ev('workflow.step_completed', { step: 'a', is_error: false }),
      ev('hitl.waiting', {
        tool_name: 'tb',
        kind: 'approval',
        workflow_skill: 'demo',
        workflow_step: 'b',
        workflow_step_index: 1,
      }),
    ]
    const blocks = foldEvents('run_wf_wait', events)
    const wf = blocks.find((b) => b.kind === 'workflow')
    if (!wf || wf.kind !== 'workflow') throw new Error('workflow block missing')
    expect(wf.steps[1].status).toBe('waiting')
    expect(wf.steps[1].waitingKind).toBe('approval')
    const card = blocks.find((b) => b.kind === 'tool')
    if (!card || card.kind !== 'tool') throw new Error('tool card missing')
    expect(card.workflowStep).toBe('b')
  })

  it('marks a workflow login wait without swapping the login card', () => {
    const base: Event[] = [
      ev('workflow.started', { skill: 'demo', steps: ['a'] }),
      ev('workflow.step_started', { step: 'a', tool: 'ta' }),
      ev('llm.tool_call', { id: 'c1', name: 'ta', arguments: {} }),
      ev('tool.result', {
        name: 'ta',
        is_error: true,
        content: { code: 'login_required', message: '需要登录', connector_id: 'c1' },
      }),
      ev('hitl.waiting', {
        tool_name: 'ta',
        kind: 'login',
        connector_id: 'c1',
        workflow_step: 'a',
      }),
    ]
    const blocks = foldEvents('run_wf_login', base)
    const wf = blocks.find((b) => b.kind === 'workflow')
    if (!wf || wf.kind !== 'workflow') throw new Error('workflow missing')
    expect(wf.steps[0]).toMatchObject({ status: 'waiting', waitingKind: 'login' })
    const card = blocks.find((b) => b.kind === 'tool')
    if (!card || card.kind !== 'tool') throw new Error('tool card missing')
    expect(card).toMatchObject({
      status: 'waiting_human',
      hitlKind: 'login',
      workflowStep: 'a',
    })
    expect(card.result).toMatchObject({ code: 'login_required' })

    const resumed = foldEvents('run_wf_login_resumed', [
      ...base,
      ev('hitl.resumed', { decision: 'approve', kind: 'login' }),
    ])
    const wfResumed = resumed.find((b) => b.kind === 'workflow')
    if (!wfResumed || wfResumed.kind !== 'workflow') throw new Error('workflow missing')
    expect(wfResumed.steps[0].status).toBe('running')
    const cardResumed = resumed.find((b) => b.kind === 'tool')
    if (!cardResumed || cardResumed.kind !== 'tool') throw new Error('tool card missing')
    expect(cardResumed.result).toBeUndefined()
  })

  it('moves the workflow step on resume and marks failure on reject/expire', () => {
    const base: Event[] = [
      ev('workflow.started', { skill: 'demo', steps: ['a', 'b'] }),
      ev('workflow.step_started', { step: 'a', tool: 'ta' }),
      ev('workflow.step_completed', { step: 'a', is_error: false }),
      ev('hitl.waiting', { tool_name: 'tb', workflow_step: 'b' }),
    ]
    const resumed = foldEvents('run_wf_resume', [
      ...base,
      ev('hitl.resumed', { decision: 'approve' }),
    ])
    const wfResumed = resumed.find((b) => b.kind === 'workflow')
    if (!wfResumed || wfResumed.kind !== 'workflow') throw new Error('workflow missing')
    expect(wfResumed.steps[1].status).toBe('running')

    const rejected = foldEvents('run_wf_reject', [
      ...base,
      ev('hitl.rejected', { decision: 'reject' }),
    ])
    const wfRejected = rejected.find((b) => b.kind === 'workflow')
    if (!wfRejected || wfRejected.kind !== 'workflow') throw new Error('workflow missing')
    expect(wfRejected.steps[1].status).toBe('failed')

    const expired = foldEvents('run_wf_expire', [
      ...base,
      ev('hitl.expired', { reason: 'waiting_expired' }),
    ])
    const wfExpired = expired.find((b) => b.kind === 'workflow')
    if (!wfExpired || wfExpired.kind !== 'workflow') throw new Error('workflow missing')
    expect(wfExpired.steps[1].status).toBe('failed')
  })

  it('does not touch workflow blocks for a normal ReAct approval', () => {
    const events: Event[] = [
      ev('workflow.started', { skill: 'demo', steps: ['a'] }),
      ev('hitl.waiting', { tool_name: 'write', prompt: 'Approve tool write?' }),
    ]
    const blocks = foldEvents('run_react', events)
    const wf = blocks.find((b) => b.kind === 'workflow')
    if (!wf || wf.kind !== 'workflow') throw new Error('workflow missing')
    expect(wf.steps[0].status).toBe('pending')
  })

  it('folds spawn and receipt into one subagent block', () => {
    const events: Event[] = [
      ev('llm.tool_call', {
        id: 'c1',
        name: 'spawn_group_run',
        arguments: { group_id: 'finance', goal: '核对发票' },
      }),
      ev('run.spawned', {
        child_run_id: 'run_child',
        group_id: 'finance',
        mode: 'readonly',
        goal: '核对发票',
        async: true,
      }),
      ev('hitl.waiting', { kind: 'waiting_child', tool_name: 'spawn_group_run' }),
      ev('run.receipt', {
        child_run_id: 'run_child',
        status: 'succeeded',
        summary: '税额合计 1240',
        facts: { total_tax: '1240.00' },
        group_id: 'finance',
        mode: 'readonly',
      }),
      ev('tool.result', {
        name: 'spawn_group_run',
        content: { status: 'succeeded', child_run_id: 'run_child', summary: '税额合计 1240' },
        is_error: false,
      }),
    ]
    const blocks = foldEvents('run_parent', events)
    const tools = blocks.filter((b) => b.kind === 'tool')
    expect(tools).toHaveLength(0)
    const sub = blocks.find((b) => b.kind === 'subagent')
    if (!sub || sub.kind !== 'subagent') throw new Error('subagent block missing')
    expect(sub).toMatchObject({
      childRunId: 'run_child',
      goal: '核对发票',
      groupId: 'finance',
      mode: 'readonly',
      status: 'succeeded',
      summary: '税额合计 1240',
      facts: { total_tax: '1240.00' },
      async: true,
    })
  })

  it('tracks subagent waiting and resumed events', () => {
    const base: Event[] = [
      ev('run.spawned', {
        child_run_id: 'run_child',
        group_id: 'finance',
        mode: 'readonly',
        goal: '核对发票',
      }),
    ]
    const waiting = foldEvents('run_parent_wait', [
      ...base,
      ev('run.child_waiting', {
        child_run_id: 'run_child',
        kind: 'approval',
        tool_name: 'create_ticket',
      }),
    ])
    const wf = waiting.find((b) => b.kind === 'subagent')
    if (!wf || wf.kind !== 'subagent') throw new Error('subagent missing')
    expect(wf.status).toBe('waiting')
    expect(wf.waitingKind).toBe('approval')

    const resumed = foldEvents('run_parent_resume', [
      ...base,
      ev('run.child_waiting', { child_run_id: 'run_child', kind: 'login', tool_name: 't' }),
      ev('run.child_resumed', { child_run_id: 'run_child' }),
    ])
    const wr = resumed.find((b) => b.kind === 'subagent')
    if (!wr || wr.kind !== 'subagent') throw new Error('subagent missing')
    expect(wr.status).toBe('running')
    expect(wr.waitingKind).toBeUndefined()
  })

  it('keeps a failed spawn as a normal tool card', () => {
    const blocks = foldEvents('run_spawn_fail', [
      ev('llm.tool_call', { id: 'c1', name: 'spawn_group_run', arguments: {} }),
      ev('tool.result', {
        name: 'spawn_group_run',
        is_error: true,
        content: { error: 'scoped_subrun_enabled is false' },
      }),
    ])
    expect(blocks.filter((b) => b.kind === 'subagent')).toHaveLength(0)
    const card = blocks.find((b) => b.kind === 'tool')
    if (!card || card.kind !== 'tool') throw new Error('tool card missing')
    expect(card.status).toBe('failed')
  })

  it('does not attach usage when nothing reported tokens or savings', () => {
    const blocks = foldEvents('run_plain', [
      ev('llm.message', { content: '无用量' }),
    ])
    expect(blocks).toEqual([{ kind: 'assistant', text: '无用量', streaming: false }])
  })
})
