import { describe, expect, it } from 'vitest'
import {
  focusActivityStepIndex,
  partitionHistoryActivity,
  partitionLiveActivity,
  shouldCollapseActivitySteps,
} from './activitySteps'
import type { ChatBlock } from './foldEvents'
import type { HistoryBlock } from './historyBlocks'

describe('partitionLiveActivity', () => {
  it('folds thinking between tools into one steps segment', () => {
    const blocks: ChatBlock[] = [
      { kind: 'assistant', text: 'hi' },
      {
        kind: 'tool',
        name: 'a',
        status: 'succeeded',
        runId: 'r',
      },
      { kind: 'thinking', turn: 1, text: 't', status: 'done', collapsed: true },
      {
        kind: 'tool',
        name: 'b',
        status: 'running',
        runId: 'r',
      },
      { kind: 'assistant', text: 'done', streaming: false },
    ]
    const segs = partitionLiveActivity(blocks)
    expect(segs.map((s) => s.type)).toEqual(['chrome', 'steps', 'chrome'])
    if (segs[1].type !== 'steps') throw new Error('expected steps')
    expect(segs[1].stepBlocks.map((b) => b.kind === 'tool' && b.name)).toEqual(['a', 'b'])
    expect(segs[1].items).toHaveLength(3)
  })
})

describe('partitionHistoryActivity', () => {
  it('keeps thinking inside the tool strip', () => {
    const blocks: HistoryBlock[] = [
      { kind: 'thinking', turn: 0, text: 'x', status: 'done', collapsed: true },
      {
        kind: 'tool',
        name: 'a',
        status: 'succeeded',
        runId: 'r',
      },
      {
        kind: 'workflow',
        skill: 's',
        runId: 'r',
        steps: [{ id: '1', status: 'done' }],
      },
    ]
    const segs = partitionHistoryActivity(blocks)
    expect(segs).toHaveLength(1)
    expect(segs[0].type).toBe('steps')
    if (segs[0].type === 'steps') {
      expect(segs[0].stepBlocks).toHaveLength(2)
      expect(segs[0].items).toHaveLength(3)
    }
  })
})

describe('focusActivityStepIndex', () => {
  it('prefers waiting_human then running then latest', () => {
    expect(
      focusActivityStepIndex([
        { kind: 'tool', name: 'a', status: 'succeeded', runId: 'r' },
        { kind: 'tool', name: 'b', status: 'waiting_human', runId: 'r' },
        { kind: 'tool', name: 'c', status: 'running', runId: 'r' },
      ]),
    ).toBe(1)
  })
})

describe('shouldCollapseActivitySteps', () => {
  it('collapses only when there are 2+ tool/workflow steps', () => {
    expect(shouldCollapseActivitySteps([{ kind: 'tool', name: 'a', status: 'running', runId: 'r' }])).toBe(
      false,
    )
    expect(
      shouldCollapseActivitySteps([
        { kind: 'tool', name: 'a', status: 'succeeded', runId: 'r' },
        { kind: 'tool', name: 'b', status: 'running', runId: 'r' },
      ]),
    ).toBe(true)
  })
})
