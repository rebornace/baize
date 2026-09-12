import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { ChatBlock } from '../foldEvents'
import { ToolCard } from './ToolCard'

const catalog = [
  { name: 'query_order', title: '查询订单', description: '按单号查询' },
]

// 若真实 tool 块字段更多，按 foldEvents.ts 的真实结构补全；这里只覆盖用到的字段。
const tool = (over: Partial<Extract<ChatBlock, { kind: 'tool' }>> = {}) =>
  ({
    kind: 'tool',
    name: 'query_order',
    status: 'running',
    runId: 'r1',
    ...over,
  }) as Extract<ChatBlock, { kind: 'tool' }>

describe('ToolCard', () => {
  it('shows a friendly phrase and hides technical name when collapsed', () => {
    const html = renderToStaticMarkup(<ToolCard block={tool()} catalog={catalog} />)
    expect(html).toContain('正在处理：查询订单')
    expect(html).not.toContain('query_order')
  })

  it('falls back to technical name when not in catalog', () => {
    const html = renderToStaticMarkup(<ToolCard block={tool({ name: 'zzz' })} catalog={[]} />)
    expect(html).toContain('zzz')
  })

  it('renders 需要你确认 with 同意/拒绝 when waiting (interactive)', () => {
    const html = renderToStaticMarkup(
      <ToolCard block={tool({ status: 'waiting_human' })} catalog={catalog} />,
    )
    expect(html).toContain('需要你确认')
    expect(html).toContain('同意')
    expect(html).toContain('拒绝')
  })

  it('readOnly renders no action buttons for historical HITL', () => {
    const html = renderToStaticMarkup(
      <ToolCard block={tool({ status: 'waiting_human' })} catalog={catalog} readOnly />,
    )
    expect(html).not.toContain('同意')
    expect(html).toContain('待确认')
  })
})
