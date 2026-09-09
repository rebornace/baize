import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { ChatBlock } from '../foldEvents'
import { WorkflowCard } from './WorkflowCard'

const wf = (
  steps: Array<{ id: string; status: 'pending' | 'running' | 'done' | 'failed' }>,
) =>
  ({
    kind: 'workflow',
    skill: 's',
    runId: 'r',
    steps,
  }) as Extract<ChatBlock, { kind: 'workflow' }>

describe('WorkflowCard', () => {
  it('shows 第 n/共 m 步 and numbered steps without raw ids', () => {
    const html = renderToStaticMarkup(
      <WorkflowCard
        block={wf([
          { id: 'a', status: 'done' },
          { id: 'b', status: 'running' },
          { id: 'c', status: 'pending' },
        ])}
      />,
    )
    expect(html).toContain('第 2 / 共 3 步')
    expect(html).toContain('步骤 1')
  })
})
