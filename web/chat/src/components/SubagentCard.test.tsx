// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import type { ChatBlock } from '../foldEvents'
import { SubagentCard } from './SubagentCard'

const block: Extract<ChatBlock, { kind: 'subagent' }> = {
  kind: 'subagent',
  childRunId: 'run_child',
  goal: '核对发票',
  groupId: 'finance',
  mode: 'readonly',
  status: 'running',
  runId: 'run_parent',
}

let host: HTMLDivElement
let root: Root

beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
})

afterEach(() => {
  act(() => {
    root?.unmount()
  })
  host.remove()
  vi.restoreAllMocks()
})

function render(el: ReactNode) {
  act(() => {
    root = createRoot(host)
    root.render(el)
  })
}

function buttonByText(text: string): HTMLButtonElement | undefined {
  return Array.from(host.querySelectorAll('button')).find(
    (b) => b.textContent?.trim() === text,
  ) as HTMLButtonElement | undefined
}

describe('SubagentCard', () => {
  it('shows goal, status, summary and facts', () => {
    render(
      <SubagentCard
        block={{
          ...block,
          status: 'succeeded',
          summary: '税额合计 1240',
          facts: { total_tax: '1240.00' },
        }}
      />,
    )
    expect(host.textContent).toContain('核对发票')
    expect(host.textContent).toContain('已完成')
    expect(host.textContent).toContain('税额合计 1240')
    expect(host.textContent).toContain('1240.00')
  })

  it('loads the child trace on expand', async () => {
    const events = vi.spyOn(api, 'listEvents').mockResolvedValue([
      { type: 'llm.tool_call', timestamp: 't1', data: { name: 'expense_query' } },
      { type: 'tool.result', timestamp: 't2', data: { name: 'expense_query', is_error: false } },
      { type: 'llm.message', timestamp: 't3', data: { content: '查到三张发票' } },
    ])
    vi.spyOn(api, 'getRunBinding').mockResolvedValue({
      run_id: 'run_child',
      binding: { model: 'test' },
    })
    render(<SubagentCard block={block} />)
    await act(async () => {
      host.querySelector<HTMLButtonElement>('[data-testid="subagent-toggle"]')?.click()
    })
    expect(events).toHaveBeenCalledWith('run_child')
    expect(host.textContent).toContain('expense_query')
    expect(host.textContent).toContain('查到三张发票')
  })

  it('approves a waiting child from the card', async () => {
    vi.spyOn(api, 'listEvents').mockResolvedValue([
      {
        type: 'hitl.waiting',
        timestamp: 't1',
        data: { kind: 'approval', tool_name: 'create_ticket', prompt: 'approve?' },
      },
    ])
    vi.spyOn(api, 'getRunBinding').mockResolvedValue({ run_id: 'run_child', binding: {} })
    const resume = vi
      .spyOn(api, 'resumeRun')
      .mockResolvedValue({ run_id: 'run_child', status: 'running' })

    render(<SubagentCard block={{ ...block, status: 'waiting', waitingKind: 'approval' }} />)
    await act(async () => {
      host.querySelector<HTMLButtonElement>('[data-testid="subagent-toggle"]')?.click()
    })
    await act(async () => {
      buttonByText('同意')?.click()
    })
    expect(resume).toHaveBeenCalledWith('run_child', 'approve', '')
  })

  it('fails open when the trace cannot be loaded', async () => {
    vi.spyOn(api, 'listEvents').mockRejectedValue(new Error('network'))
    vi.spyOn(api, 'getRunBinding').mockRejectedValue(new Error('network'))
    render(<SubagentCard block={block} />)
    await act(async () => {
      host.querySelector<HTMLButtonElement>('[data-testid="subagent-toggle"]')?.click()
    })
    expect(host.textContent).toContain('轨迹暂不可用')
  })
})
