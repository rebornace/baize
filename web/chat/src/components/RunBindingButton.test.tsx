// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../api/http'
import { RunBindingButton } from './RunBindingButton'

vi.mock('../api', () => ({
  getRunBinding: vi.fn(),
  getRun: vi.fn(),
  listRunChildren: vi.fn(),
}))

import { getRun, getRunBinding, listRunChildren } from '../api'

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.mocked(getRunBinding).mockReset()
  vi.mocked(getRun).mockReset()
  vi.mocked(listRunChildren).mockReset()
  vi.mocked(getRun).mockResolvedValue({
    id: 'r1',
    agent_id: 'a',
    input: 'x',
    status: 'succeeded',
    created_at: '2026-01-01T00:00:00Z',
  })
  vi.mocked(listRunChildren).mockResolvedValue([])
})
afterEach(() => {
  host.remove()
})

function render(el: ReactNode) {
  act(() => {
    createRoot(host).render(el)
  })
}

describe('RunBindingButton', () => {
  it('defaults a missing origin to manual for old runs', async () => {
    vi.mocked(getRunBinding).mockResolvedValue({ run_id: 'r1', binding: { model_profile_id: 'mp1' } })
    vi.mocked(getRun).mockResolvedValue({
      id: 'r1',
      agent_id: 'a',
      input: 'x',
      status: 'succeeded',
      created_at: '2026-01-01T00:00:00Z',
    })
    render(<RunBindingButton runId="r1" />)
    const btn = host.querySelector('[data-testid="run-binding-open"]') as HTMLButtonElement
    await act(async () => {
      btn.click()
      await new Promise((r) => setTimeout(r, 0))
    })
    const origin = host.querySelector('[data-testid="run-binding-origin"]')
    expect(origin?.textContent).toContain('手动')
  })

  it('shows scheduled origin in the modal', async () => {
    vi.mocked(getRunBinding).mockResolvedValue({ run_id: 'r1', binding: { model_profile_id: 'mp1' } })
    vi.mocked(getRun).mockResolvedValue({
      id: 'r1',
      agent_id: 'a',
      input: 'x',
      status: 'succeeded',
      created_at: '2026-01-01T00:00:00Z',
      origin: 'scheduled',
    })
    render(<RunBindingButton runId="r1" />)
    const btn = host.querySelector('[data-testid="run-binding-open"]') as HTMLButtonElement
    await act(async () => {
      btn.click()
      await new Promise((r) => setTimeout(r, 0))
    })
    const origin = host.querySelector('[data-testid="run-binding-origin"]')
    expect(origin?.textContent).toContain('定时')
  })

  it('shows token usage in the modal', async () => {
    vi.mocked(getRunBinding).mockResolvedValue({
      run_id: 'r1',
      binding: { model_profile_id: 'mp1' },
      usage: {
        turns: 3,
        prompt_tokens: 100,
        completion_tokens: 20,
        total_tokens: 120,
        cached_tokens: 40,
        priced: true,
        cost_usd: 0.000123,
        currency: 'USD',
      },
    })
    render(<RunBindingButton runId="r1" />)
    const btn = host.querySelector('[data-testid="run-binding-open"]') as HTMLButtonElement
    await act(async () => {
      btn.click()
      await new Promise((r) => setTimeout(r, 0))
    })
    const usage = host.querySelector('[data-testid="run-binding-usage"]')
    expect(usage).toBeTruthy()
    expect(usage?.textContent).toContain('输入 100')
    expect(usage?.textContent).toContain('合计 120')
    expect(usage?.textContent).toContain('3 轮')
    expect(usage?.textContent).toContain('缓存命中 40')
    const cost = host.querySelector('[data-testid="run-binding-cost"]')
    expect(cost?.textContent).toContain('USD 0.000123')
  })

  it('opens modal with pretty JSON on success', async () => {
    vi.mocked(getRunBinding).mockResolvedValue({
      run_id: 'r1',
      binding: { model_profile_id: 'mp1', knobs: { thinking: 'high' } },
    })
    render(<RunBindingButton runId="r1" />)
    const btn = host.querySelector('[data-testid="run-binding-open"]') as HTMLButtonElement
    expect(btn).not.toBeNull()
    await act(async () => {
      btn.click()
    })
    expect(getRunBinding).toHaveBeenCalledWith('r1')
    const body = host.querySelector('[data-testid="run-binding-body"]')
    expect(body?.textContent).toContain('model_profile_id')
    expect(body?.textContent).toContain('mp1')
  })

  it('hides itself on 404 binding_not_found', async () => {
    vi.mocked(getRunBinding).mockRejectedValue(
      new ApiError(404, 'binding_not_found', 'missing'),
    )
    render(<RunBindingButton runId="r-missing" />)
    const btn = host.querySelector('[data-testid="run-binding-open"]') as HTMLButtonElement
    await act(async () => {
      btn.click()
    })
    expect(host.querySelector('[data-testid="run-binding-open"]')).toBeNull()
    expect(host.querySelector('[role="dialog"]')).toBeNull()
  })

  it('renders nothing for empty runId', () => {
    render(<RunBindingButton runId="  " />)
    expect(host.querySelector('[data-testid="run-binding-open"]')).toBeNull()
  })

  it('lists children and switches binding to a child run', async () => {
    vi.mocked(getRunBinding).mockImplementation(async (id: string) => ({
      run_id: id,
      binding: { run: id },
    }))
    vi.mocked(getRun).mockImplementation(async (id: string) => {
      if (id === 'child-1') {
        return {
          id: 'child-1',
          agent_id: 'a',
          input: 'c',
          status: 'succeeded',
          created_at: '2026-01-01T00:00:01Z',
          parent_run_id: 'r1',
          depth: 1,
          group_id: 'finance',
          spawn_mode: 'readonly',
        }
      }
      return {
        id: 'r1',
        agent_id: 'a',
        input: 'p',
        status: 'succeeded',
        created_at: '2026-01-01T00:00:00Z',
      }
    })
    vi.mocked(listRunChildren).mockResolvedValue([
      {
        id: 'child-1',
        agent_id: 'a',
        input: 'c',
        status: 'succeeded',
        created_at: '2026-01-01T00:00:01Z',
        parent_run_id: 'r1',
        depth: 1,
        group_id: 'finance',
        spawn_mode: 'readonly',
      },
    ])

    render(<RunBindingButton runId="r1" />)
    const btn = host.querySelector('[data-testid="run-binding-open"]') as HTMLButtonElement
    await act(async () => {
      btn.click()
    })

    expect(host.querySelector('[data-testid="run-binding-nav"]')).not.toBeNull()
    expect(host.querySelector('[data-testid="run-binding-body"]')?.textContent).toContain('"run": "r1"')

    const childBtn = host.querySelector(
      '[data-testid="run-binding-open-child-child-1"]',
    ) as HTMLButtonElement
    expect(childBtn).not.toBeNull()
    await act(async () => {
      childBtn.click()
    })

    expect(getRunBinding).toHaveBeenCalledWith('child-1')
    expect(host.querySelector('[data-testid="run-binding-body"]')?.textContent).toContain(
      '"run": "child-1"',
    )
    expect(host.querySelector('[data-testid="run-binding-open-parent"]')).not.toBeNull()
  })

  it('opens parent binding from a child run', async () => {
    vi.mocked(getRunBinding).mockImplementation(async (id: string) => ({
      run_id: id,
      binding: { run: id },
    }))
    vi.mocked(getRun).mockImplementation(async (id: string) => {
      if (id === 'child-1' || id.startsWith('child')) {
        return {
          id: 'child-1',
          agent_id: 'a',
          input: 'c',
          status: 'succeeded',
          created_at: '2026-01-01T00:00:01Z',
          parent_run_id: 'parent-1',
          depth: 1,
          group_id: 'commerce',
          spawn_mode: 'readwrite',
        }
      }
      return {
        id: 'parent-1',
        agent_id: 'a',
        input: 'p',
        status: 'succeeded',
        created_at: '2026-01-01T00:00:00Z',
      }
    })
    vi.mocked(listRunChildren).mockResolvedValue([
      {
        id: 'child-1',
        agent_id: 'a',
        input: 'c',
        status: 'succeeded',
        created_at: '2026-01-01T00:00:01Z',
        parent_run_id: 'parent-1',
        depth: 1,
        group_id: 'commerce',
        spawn_mode: 'readwrite',
      },
    ])

    render(<RunBindingButton runId="child-1" />)
    const btn = host.querySelector('[data-testid="run-binding-open"]') as HTMLButtonElement
    await act(async () => {
      btn.click()
    })
    expect(host.querySelector('[data-testid="run-binding-open-parent"]')).not.toBeNull()

    await act(async () => {
      ;(host.querySelector('[data-testid="run-binding-open-parent"]') as HTMLButtonElement).click()
    })
    expect(getRunBinding).toHaveBeenCalledWith('parent-1')
    expect(host.querySelector('[data-testid="run-binding-body"]')?.textContent).toContain(
      '"run": "parent-1"',
    )
  })
})
