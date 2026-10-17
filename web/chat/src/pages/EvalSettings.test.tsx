// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { EvalSettings } from './EvalSettings'
import { ApiError } from '../api/http'
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

function mockSummary(overrides: Partial<evalApi.EvalCalibrationSummary> = {}) {
  return vi.spyOn(evalApi, 'getCalibrationSummary').mockResolvedValue({
    scored_count: 0,
    calibrated_count: 0,
    agree_count: 0,
    disagree_count: 0,
    agreement_rate: null,
    uncalibrated_scored_count: 0,
    ...overrides,
  })
}

async function renderPage(summary: Partial<evalApi.EvalCalibrationSummary> = {}) {
  vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({ items: [] })
  vi.spyOn(evalApi, 'queryEvalRuns').mockResolvedValue({ items: [] })
  mockSummary(summary)
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
    mockSummary()
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
    mockSummary()
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

  it('loads calibration summary with data-testid', async () => {
    await renderPage({
      scored_count: 3,
      calibrated_count: 2,
      agree_count: 1,
      disagree_count: 1,
      agreement_rate: 0.5,
      uncalibrated_scored_count: 1,
    })
    expect(evalApi.getCalibrationSummary).toHaveBeenCalled()
    const el = host.querySelector('[data-testid="eval-calibration-summary"]')
    expect(el).toBeTruthy()
    expect(host.textContent).toContain('3')
    expect(host.textContent).toContain('2')
    expect(host.textContent).toMatch(/50/)
  })

  it('shows no-calibration copy instead of 0% when nothing calibrated', async () => {
    await renderPage({ scored_count: 1, agreement_rate: null, calibrated_count: 0 })
    expect(host.textContent).toContain('尚无标定')
    expect(host.textContent).not.toMatch(/0%/)
  })

  it('shows calibration actions for scored confirmed case', async () => {
    mockSummary({ scored_count: 1 })
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({
      items: [
        {
          id: 'ec_scored',
          source_run_id: 'run_x',
          status: 'confirmed',
          input: 'case input',
          last_score_pass: true,
          last_scored_at: '2026-02-01T12:00:00Z',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
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
    expect(host.querySelector('[data-testid="eval-calibrate-agree-ec_scored"]')).toBeTruthy()
    expect(host.querySelector('[data-testid="eval-calibrate-disagree-ec_scored"]')).toBeTruthy()
    expect(host.querySelector('[data-testid="eval-calibrate-clear-ec_scored"]')).toBeTruthy()
  })

  it('does not show clickable calibration for draft or unscored', async () => {
    mockSummary()
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({
      items: [
        {
          id: 'ec_draft',
          source_run_id: 'run_d',
          status: 'draft',
          input: 'draft case',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
        {
          id: 'ec_unscored',
          source_run_id: 'run_u',
          status: 'confirmed',
          input: 'no score',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
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
    expect(host.querySelector('[data-testid="eval-calibrate-agree-ec_draft"]')).toBeNull()
    expect(host.querySelector('[data-testid="eval-calibrate-agree-ec_unscored"]')).toBeNull()
  })

  it('shows stale calibration hint when calibrated before last score', async () => {
    mockSummary({ scored_count: 1, calibrated_count: 1, agreement_rate: 1 })
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({
      items: [
        {
          id: 'ec_stale',
          source_run_id: 'run_s',
          status: 'confirmed',
          input: 'stale',
          last_score_pass: false,
          last_scored_at: '2026-03-01T12:00:00Z',
          human_agree: true,
          calibrated_at: '2026-02-01T12:00:00Z',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
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
    expect(host.querySelector('[data-testid="eval-calibration-stale-ec_stale"]')).toBeTruthy()
  })

  it('submits agree calibration with optional note', async () => {
    mockSummary({ scored_count: 1 })
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({
      items: [
        {
          id: 'ec_go',
          source_run_id: 'run_g',
          status: 'confirmed',
          input: 'go',
          last_score_pass: true,
          last_scored_at: '2026-02-01T12:00:00Z',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ],
    })
    const putSpy = vi.spyOn(evalApi, 'putCalibration').mockResolvedValue({
      id: 'ec_go',
      source_run_id: 'run_g',
      status: 'confirmed',
      input: 'go',
      last_score_pass: true,
      last_scored_at: '2026-02-01T12:00:00Z',
      human_agree: true,
      calibrated_at: '2026-02-02T12:00:00Z',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
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
    const note = host.querySelector('[data-testid="eval-calibrate-note-ec_go"]') as HTMLInputElement
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    await act(async () => {
      setter?.call(note, 'looks right')
      note.dispatchEvent(new Event('input', { bubbles: true }))
      note.dispatchEvent(new Event('change', { bubbles: true }))
      await Promise.resolve()
    })
    const agreeBtn = host.querySelector(
      '[data-testid="eval-calibrate-agree-ec_go"]',
    ) as HTMLButtonElement
    await act(async () => {
      agreeBtn.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(putSpy).toHaveBeenCalledWith('ec_go', { agree: true, note: 'looks right' })
  })

  it('refetches calibration summary after re-score', async () => {
    const summarySpy = mockSummary({ scored_count: 1 })
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({
      items: [
        {
          id: 'ec_rescore',
          source_run_id: 'run_r',
          status: 'confirmed',
          input: 'rescore me',
          last_score_pass: true,
          last_scored_at: '2026-02-01T12:00:00Z',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ],
    })
    vi.spyOn(evalApi, 'scoreEvalCase').mockResolvedValue({ pass: true, codes: [] })
    await act(async () => {
      root.render(
        <MemoryRouter>
          <EvalSettings />
        </MemoryRouter>,
      )
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(summarySpy.mock.calls.length).toBe(1)
    const rescoreBtn = Array.from(host.querySelectorAll('button')).find(
      (b) => b.textContent === '重打分',
    ) as HTMLButtonElement
    await act(async () => {
      rescoreBtn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(summarySpy.mock.calls.length).toBe(2)
    expect(evalApi.scoreEvalCase).toHaveBeenCalledWith('ec_rescore')
  })

  it('renders check-state mock input and button on corpus row', async () => {
    mockSummary()
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({
      items: [
        {
          id: 'ec_live',
          source_run_id: 'run_live',
          status: 'confirmed',
          input: 'live state case',
          forbid_tools: ['Shell'],
          expect_state: [{ status: 'open' }],
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
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
    expect(host.querySelector('[data-testid="eval-check-state-ec_live"]')).toBeTruthy()
    expect(host.querySelector('[data-testid="eval-check-state-url-ec_live"]')).toBeTruthy()
    expect(host.querySelector('[data-testid="eval-check-state-btn-ec_live"]')).toBeTruthy()
    expect(host.textContent).toContain('forbid_tools')
    expect(host.textContent).toContain('expect_state')
  })

  it('updates row score text after check-state returns codes', async () => {
    mockSummary()
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({
      items: [
        {
          id: 'ec_state',
          source_run_id: 'run_s',
          status: 'confirmed',
          input: 'state case',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ],
    })
    const checkSpy = vi.spyOn(evalApi, 'checkEvalCaseState').mockResolvedValue({
      id: 'ec_state',
      source_run_id: 'run_s',
      status: 'confirmed',
      input: 'state case',
      last_score_pass: false,
      last_score_codes: ['STATE'],
      last_scored_at: '2026-03-01T12:00:00Z',
      has_state_snapshot: true,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-03-01T00:00:00Z',
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
    expect(host.textContent).toContain('尚未重打分')
    const urlInput = host.querySelector(
      '[data-testid="eval-check-state-url-ec_state"]',
    ) as HTMLInputElement
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    await act(async () => {
      setter?.call(urlInput, 'http://127.0.0.1:9999')
      urlInput.dispatchEvent(new Event('input', { bubbles: true }))
      urlInput.dispatchEvent(new Event('change', { bubbles: true }))
      await Promise.resolve()
    })
    const btn = host.querySelector(
      '[data-testid="eval-check-state-btn-ec_state"]',
    ) as HTMLButtonElement
    await act(async () => {
      btn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(checkSpy).toHaveBeenCalledWith('ec_state', 'http://127.0.0.1:9999')
    expect(host.textContent).toMatch(/未通过/)
    expect(host.textContent).toContain('STATE')
    expect(host.textContent).not.toContain('尚未重打分')
  })

  it('clears stale score codes when check-state passes without codes', async () => {
    mockSummary()
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({
      items: [
        {
          id: 'ec_clear',
          source_run_id: 'run_c',
          status: 'confirmed',
          input: 'clear codes',
          last_score_pass: false,
          last_score_codes: ['STATE'],
          last_scored_at: '2026-03-01T10:00:00Z',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ],
    })
    vi.spyOn(evalApi, 'checkEvalCaseState').mockResolvedValue({
      id: 'ec_clear',
      source_run_id: 'run_c',
      status: 'confirmed',
      input: 'clear codes',
      last_score_pass: true,
      last_scored_at: '2026-03-01T12:00:00Z',
      has_state_snapshot: true,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-03-01T00:00:00Z',
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
    expect(host.textContent).toContain('STATE')
    const urlInput = host.querySelector(
      '[data-testid="eval-check-state-url-ec_clear"]',
    ) as HTMLInputElement
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    await act(async () => {
      setter?.call(urlInput, 'http://127.0.0.1:9999')
      urlInput.dispatchEvent(new Event('input', { bubbles: true }))
      urlInput.dispatchEvent(new Event('change', { bubbles: true }))
      await Promise.resolve()
    })
    const btn = host.querySelector(
      '[data-testid="eval-check-state-btn-ec_clear"]',
    ) as HTMLButtonElement
    await act(async () => {
      btn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(host.textContent).toMatch(/通过/)
    expect(host.textContent).not.toContain('STATE')
  })

  it('shows toast when check-state API fails', async () => {
    mockSummary()
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({
      items: [
        {
          id: 'ec_fail',
          source_run_id: 'run_f',
          status: 'confirmed',
          input: 'fail case',
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ],
    })
    vi.spyOn(evalApi, 'checkEvalCaseState').mockRejectedValue(
      new ApiError(400, 'invalid_request', 'mock base URL required'),
    )
    await act(async () => {
      root.render(
        <MemoryRouter>
          <EvalSettings />
        </MemoryRouter>,
      )
      await Promise.resolve()
      await Promise.resolve()
    })
    const urlInput = host.querySelector(
      '[data-testid="eval-check-state-url-ec_fail"]',
    ) as HTMLInputElement
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    await act(async () => {
      setter?.call(urlInput, 'http://bad')
      urlInput.dispatchEvent(new Event('input', { bubbles: true }))
      await Promise.resolve()
    })
    const btn = host.querySelector(
      '[data-testid="eval-check-state-btn-ec_fail"]',
    ) as HTMLButtonElement
    await act(async () => {
      btn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    const toast = host.querySelector('[data-testid="toast"]')
    expect(toast).toBeTruthy()
    expect(toast?.textContent).toMatch(/invalid_request|mock base URL required|请求/i)
  })
})
