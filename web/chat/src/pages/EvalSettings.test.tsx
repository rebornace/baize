// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { EvalSettings } from './EvalSettings'
import * as evalApi from '../api/eval'
import * as runsApi from '../api/runs'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'

let host: HTMLDivElement
let root: Root

beforeEach(() => {
  setPack(zhPack)
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})

afterEach(() => {
  act(() => {
    root.unmount()
  })
  host.remove()
  vi.restoreAllMocks()
})

async function renderPage() {
  vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({ items: [] })
  vi.spyOn(evalApi, 'queryEvalRuns').mockResolvedValue({ items: [] })
  await act(async () => {
    root.render(
      <MemoryRouter>
        <EvalSettings />
      </MemoryRouter>,
    )
    await Promise.resolve()
    await Promise.resolve()
  })
}

describe('EvalSettings', () => {
  it('renders filter query and candidates container', async () => {
    await renderPage()
    expect(host.querySelector('[data-testid="eval-query"]')).toBeTruthy()
    expect(host.querySelector('[data-testid="eval-candidates"]')).toBeTruthy()
    expect(host.textContent).toMatch(/evidence-eval/)
  })

  it('loads cases list container', async () => {
    await renderPage()
    expect(host.querySelector('[data-testid="eval-cases"]')).toBeTruthy()
    expect(evalApi.listEvalCases).toHaveBeenCalled()
  })

  it('queries runs when filter submitted', async () => {
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({ items: [] })
    const runsSpy = vi.spyOn(evalApi, 'queryEvalRuns').mockResolvedValue({
      items: [
        {
          id: 'run_1',
          agent_id: 'a',
          status: 'failed',
          created_at: '2026-01-01T00:00:00Z',
          duration_ms: 1200,
          input_preview: 'hello',
        },
      ],
    })
    await act(async () => {
      root.render(
        <MemoryRouter>
          <EvalSettings />
        </MemoryRouter>,
      )
      await Promise.resolve()
      await Promise.resolve()
    })
    const btn = host.querySelector('[data-testid="eval-query"]') as HTMLButtonElement
    await act(async () => {
      btn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(runsSpy).toHaveBeenCalled()
    expect(host.textContent).toContain('run_1')
  })

  it('opens run events from candidate row', async () => {
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({ items: [] })
    vi.spyOn(evalApi, 'queryEvalRuns').mockResolvedValue({
      items: [
        {
          id: 'run_ev',
          agent_id: 'a',
          status: 'failed',
          created_at: '2026-01-01T00:00:00Z',
          input_preview: 'x',
        },
      ],
    })
    const eventsSpy = vi.spyOn(runsApi, 'listEvents').mockResolvedValue([
      { type: 'tool.result', timestamp: '2026-01-01T00:00:01Z', data: { is_error: true } },
    ])
    await act(async () => {
      root.render(
        <MemoryRouter>
          <EvalSettings />
        </MemoryRouter>,
      )
      await Promise.resolve()
      await Promise.resolve()
    })
    const queryBtn = host.querySelector('[data-testid="eval-query"]') as HTMLButtonElement
    await act(async () => {
      queryBtn.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    const viewBtn = host.querySelector('[data-testid="eval-view-events-run_ev"]') as HTMLButtonElement
    await act(async () => {
      viewBtn.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(eventsSpy).toHaveBeenCalledWith('run_ev')
    expect(host.textContent).toContain('tool.result')
  })
})
