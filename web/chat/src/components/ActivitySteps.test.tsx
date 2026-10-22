import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { ActivityStepBlock } from '../activitySteps'
import { ActivitySteps } from './ActivitySteps'

const tools = (n: number, runningAt = n - 1): ActivityStepBlock[] =>
  Array.from({ length: n }, (_, i) => ({
    kind: 'tool' as const,
    name: `t${i}`,
    status: i === runningAt ? ('running' as const) : ('succeeded' as const),
    runId: 'r',
  }))

describe('ActivitySteps', () => {
  it('single step renders children without strip chrome', () => {
    const html = renderToStaticMarkup(
      <ActivitySteps blocks={tools(1)} catalog={[]}>
        <div data-one>only</div>
      </ActivitySteps>,
    )
    expect(html).toContain('only')
    expect(html).not.toContain('activity-steps')
    expect(html).not.toContain('展开步骤')
  })

  it('multi step collapses to one bar showing progress and focus', () => {
    const html = renderToStaticMarkup(
      <ActivitySteps blocks={tools(3, 1)} catalog={[{ name: 't1', title: '查库' }]}>
        <div>card-a</div>
        <div>card-b</div>
        <div>card-c</div>
      </ActivitySteps>,
    )
    expect(html).toContain('activity-steps')
    expect(html).toContain('data-expanded="false"')
    expect(html).toContain('第 2 / 共 3 步')
    expect(html).toContain('正在处理：查库')
    expect(html).toContain('展开步骤')
    expect(html).not.toContain('card-a')
  })

  it('stays collapsed when a step needs confirmation', () => {
    const blocks: ActivityStepBlock[] = [
      { kind: 'tool', name: 'a', status: 'succeeded', runId: 'r' },
      { kind: 'tool', name: 'b', status: 'waiting_human', runId: 'r' },
    ]
    const html = renderToStaticMarkup(
      <ActivitySteps blocks={blocks} catalog={[]}>
        <div>card-waiting</div>
      </ActivitySteps>,
    )
    expect(html).toContain('data-expanded="false"')
    expect(html).toContain('需要你确认')
    expect(html).not.toContain('card-waiting')
  })
})
