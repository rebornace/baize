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

const runStub: api.Run = {
  id: 'run_child',
  agent_id: 'a',
  input: 'child',
  status: 'running',
  created_at: '2026-11-20T00:00:00Z',
}

beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.spyOn(api, 'getRun').mockResolvedValue(runStub)
  vi.spyOn(api, 'getRunBinding').mockResolvedValue({ run_id: 'run_child', binding: {} })
  vi.spyOn(api, 'listEvents').mockResolvedValue([])
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
          async: true,
        }}
      />,
    )
    expect(host.textContent).toContain('核对发票')
    expect(host.textContent).toContain('已完成')
    expect(host.textContent).toContain('异步')
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

  it('fills summary and facts from the parent receipt when the child already finished', async () => {
    vi.spyOn(api, 'getRun').mockResolvedValue({ ...runStub, status: 'succeeded' })
    vi.spyOn(api, 'listEvents').mockImplementation(async (id) =>
      id === block.runId
        ? [
            {
              type: 'run.receipt',
              timestamp: 't1',
              data: {
                child_run_id: block.childRunId,
                status: 'succeeded',
                summary: '税额合计 1240',
                facts: { total_tax: '1240.00' },
              },
            },
          ]
        : [],
    )
    render(<SubagentCard block={{ ...block, async: true, status: 'running' }} />)
    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(host.textContent).toContain('税额合计 1240')
    expect(host.textContent).toContain('1240.00')
  })

  it('offers continue for an async terminal child and prefills a receipt prompt', () => {
    const onContinue = vi.fn()
    render(
      <SubagentCard
        block={{
          ...block,
          async: true,
          status: 'succeeded',
          summary: '税额合计 1240',
          facts: { total_tax: '1240.00' },
        }}
        onContinue={onContinue}
      />,
    )
    const btn = buttonByText('继续处理')
    expect(btn).toBeTruthy()
    act(() => {
      btn!.click()
    })
    expect(onContinue).toHaveBeenCalledTimes(1)
    const prompt = onContinue.mock.calls[0][0] as string
    expect(prompt).toContain('税额合计 1240')
    expect(prompt).toContain('total_tax')
    expect(prompt).toContain('继续处理')
  })

  it('disables continue while a run is active', () => {
    render(
      <SubagentCard
        block={{ ...block, async: true, status: 'succeeded', summary: 'ok' }}
        onContinue={() => undefined}
        continueDisabled
      />,
    )
    expect(buttonByText('继续处理')!.disabled).toBe(true)
    expect(host.textContent).toContain('当前回复结束后可继续')
  })

  it('does not offer continue for a sync child', () => {
    render(
      <SubagentCard
        block={{ ...block, status: 'succeeded', summary: 'ok' }}
        onContinue={() => undefined}
      />,
    )
    expect(buttonByText('继续处理')).toBeUndefined()
  })

  it('cancels a child only after a second confirmation', async () => {
    const cancel = vi
      .spyOn(api, 'cancelRun')
      .mockResolvedValue({ run_id: 'run_child', status: 'cancelled' })
    render(<SubagentCard block={block} />)
    await act(async () => {
      host.querySelector<HTMLButtonElement>('[data-testid="subagent-toggle"]')?.click()
    })
    act(() => {
      buttonByText('停止子代理')!.click()
    })
    expect(cancel).not.toHaveBeenCalled()
    await act(async () => {
      buttonByText('确认停止')!.click()
    })
    expect(cancel).toHaveBeenCalledWith('run_child')
  })

  it('auto refreshes an expanded running child', async () => {
    vi.useFakeTimers()
    const events = vi.spyOn(api, 'listEvents').mockResolvedValue([])
    try {
      render(<SubagentCard block={block} />)
      await act(async () => {
        host.querySelector<HTMLButtonElement>('[data-testid="subagent-toggle"]')?.click()
      })
      const before = events.mock.calls.length
      await act(async () => {
        await vi.advanceTimersByTimeAsync(3000)
      })
      expect(events.mock.calls.length).toBeGreaterThan(before)
    } finally {
      vi.useRealTimers()
    }
  })

  it('uses the remote child status when the parent event is missing', async () => {
    vi.spyOn(api, 'getRun').mockResolvedValue({ ...runStub, status: 'succeeded' })
    render(<SubagentCard block={block} />)
    await act(async () => {
      host.querySelector<HTMLButtonElement>('[data-testid="subagent-toggle"]')?.click()
    })
    expect(host.textContent).toContain('已完成')
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
