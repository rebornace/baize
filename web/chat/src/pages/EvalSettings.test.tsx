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
  vi.spyOn(evalApi, 'getEvalGate').mockResolvedValue({ min_pass_rate: 0.9, last: null })
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
  it('runs the regression gate and lists failures', async () => {
    const runSpy = vi.spyOn(evalApi, 'runEvalGate').mockResolvedValue({
      total: 2,
      passed: 1,
      failed: 1,
      pass_rate: 0.5,
      min_pass_rate: 0.9,
      pass: false,
      empty: false,
      ran_at: '2026-11-24T03:00:00Z',
      failures: [{ id: 'ec_bad', input: 'bad case', codes: ['BSR'], notes: 'missing_reads:a' }],
    })
    await renderPage()
    expect(host.textContent).toContain('还没有运行过回归')
    const btn = host.querySelector('[data-testid="eval-gate-run"]') as HTMLButtonElement
    expect(btn).not.toBeNull()
    await act(async () => {
      btn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(runSpy).toHaveBeenCalled()
    const result = host.querySelector('[data-testid="eval-gate-result"]')
    expect(result?.textContent).toContain('门禁未通过')
    expect(result?.textContent).toContain('通过 1/2（50%），阈值 90%')
    expect(result?.textContent).toContain('ec_bad')
    expect(result?.textContent).toContain('missing_reads:a')
  })

  it('renders filter query and candidates container', async () => {
    await renderPage()
    expect(host.querySelector('[data-testid="eval-query"]')).toBeTruthy()
    expect(host.querySelector('[data-testid="eval-sample"]')).toBeTruthy()
    expect(host.querySelector('[data-testid="eval-candidates"]')).toBeTruthy()
    expect(host.textContent).toMatch(/评测脚本|developers|corpus|语料/)
  })

  it('samples into corpus from filter', async () => {
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({ items: [] })
    mockSummary()
    const sampleSpy = vi.spyOn(evalApi, 'sampleEvalRuns').mockResolvedValue({
      mode: 'draft',
      candidates: 2,
      sampled: 2,
      created: 1,
      skipped: 1,
      scored: 0,
      errors: [],
      case_ids: ['ec_1'],
    })
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
    const btn = host.querySelector('[data-testid="eval-sample"]') as HTMLButtonElement
    await act(async () => {
      btn.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(sampleSpy).toHaveBeenCalled()
    const arg = sampleSpy.mock.calls[0]?.[0] as { mode?: string }
    expect(arg.mode).toBe('draft')
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

  it('renders check-state endpoint input and button on corpus row', async () => {
    mockSummary()
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({
      items: [
        {
          id: 'ec_live',
          source_run_id: 'run_live',
          status: 'confirmed',
          input: 'live state case',
          forbid_tools: ['Shell'],
          expect_state: [{ fields: { status: { op: 'eq', value: 'open' } } }],
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

  async function renderWithCandidateRun(runId = 'run_skill') {
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({ items: [] })
    mockSummary()
    vi.spyOn(evalApi, 'queryEvalRuns').mockResolvedValue({
      items: [
        {
          id: runId,
          agent_id: 'a',
          status: 'failed',
          created_at: '2026-01-01T00:00:00Z',
          duration_ms: 500,
          input_preview: 'skill case',
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
    const queryBtn = host.querySelector('[data-testid="eval-query"]') as HTMLButtonElement
    await act(async () => {
      queryBtn.click()
      await Promise.resolve()
      await Promise.resolve()
    })
  }

  it('opens skill draft modal from dig-skill button', async () => {
    const draftSpy = vi.spyOn(evalApi, 'fetchSkillDraft').mockResolvedValue({
      source_run_id: 'run_skill',
      suggested_id: 'from-run-run_skill',
      markdown: '---\nname: from-run-run_skill\ndescription: test\n---\n\n# Steps',
      timeline: [
        {
          run_id: 'run_skill',
          type: 'tool.result',
          timestamp: '2026-01-01T00:00:01Z',
          summary: 'Shell failed',
        },
      ],
    })
    await renderWithCandidateRun()
    const digBtn = host.querySelector(
      '[data-testid="eval-dig-skill-run_skill"]',
    ) as HTMLButtonElement
    expect(digBtn).toBeTruthy()
    await act(async () => {
      digBtn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(draftSpy).toHaveBeenCalledWith('run_skill')
    const ta = host.querySelector('[data-testid="eval-skill-draft-markdown"]') as HTMLTextAreaElement
    expect(ta).toBeTruthy()
    expect(ta.value).toContain('---')
    expect(host.textContent).toContain('tool.result')
  })

  it('installs skill draft on confirm', async () => {
    vi.spyOn(evalApi, 'fetchSkillDraft').mockResolvedValue({
      source_run_id: 'run_skill',
      suggested_id: 'from-run-run_skill',
      markdown: '---\nname: from-run-run_skill\ndescription: test\n---\n\nbody',
      timeline: [],
    })
    const installSpy = vi.spyOn(evalApi, 'installSkillDraft').mockResolvedValue({
      id: 'from-run-run_skill',
    })
    await renderWithCandidateRun()
    const digBtn = host.querySelector(
      '[data-testid="eval-dig-skill-run_skill"]',
    ) as HTMLButtonElement
    await act(async () => {
      digBtn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    const installBtn = Array.from(host.querySelectorAll('button')).find(
      (b) => b.textContent === '安装为技能',
    ) as HTMLButtonElement
    expect(installBtn).toBeTruthy()
    await act(async () => {
      installBtn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(installSpy).toHaveBeenCalledWith('run_skill', {
      markdown: expect.stringContaining('---'),
    })
    const toast = host.querySelector('[data-testid="toast"]')
    expect(toast).toBeTruthy()
    expect(toast?.textContent).toMatch(/已安装|Skill/)
  })

  it('keeps markdown when polish fails', async () => {
    vi.spyOn(evalApi, 'fetchSkillDraft').mockResolvedValue({
      source_run_id: 'run_skill',
      suggested_id: 'from-run-run_skill',
      markdown: '---\nname: from-run-run_skill\ndescription: test\n---\n\nORIGINAL',
      timeline: [],
    })
    vi.spyOn(evalApi, 'polishSkillDraft').mockRejectedValue(new Error('llm down'))
    await renderWithCandidateRun()
    const digBtn = host.querySelector(
      '[data-testid="eval-dig-skill-run_skill"]',
    ) as HTMLButtonElement
    await act(async () => {
      digBtn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    const ta = host.querySelector('[data-testid="eval-skill-draft-markdown"]') as HTMLTextAreaElement
    expect(ta.value).toContain('ORIGINAL')
    const polishBtn = host.querySelector('[data-testid="skill-draft-polish"]') as HTMLButtonElement
    await act(async () => {
      polishBtn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(ta.value).toContain('ORIGINAL')
    expect(host.textContent).toMatch(/润色失败|Polish failed/)
  })

  it('passes create_eval_stub when checkbox checked', async () => {
    vi.spyOn(evalApi, 'fetchSkillDraft').mockResolvedValue({
      source_run_id: 'run_skill',
      suggested_id: 'from-run-run_skill',
      markdown: '---\nname: from-run-run_skill\ndescription: test\n---\n\nbody',
      timeline: [],
    })
    const installSpy = vi.spyOn(evalApi, 'installSkillDraft').mockResolvedValue({
      id: 'from-run-run_skill',
      eval_stub: 'created',
    })
    await renderWithCandidateRun()
    const digBtn = host.querySelector(
      '[data-testid="eval-dig-skill-run_skill"]',
    ) as HTMLButtonElement
    await act(async () => {
      digBtn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    const stub = host.querySelector('[data-testid="skill-draft-eval-stub"]') as HTMLInputElement
    await act(async () => {
      stub.click()
      await Promise.resolve()
    })
    const installBtn = host.querySelector('[data-testid="skill-draft-install"]') as HTMLButtonElement
    await act(async () => {
      installBtn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(installSpy).toHaveBeenCalledWith('run_skill', {
      markdown: expect.stringContaining('---'),
      create_eval_stub: true,
    })
  })
})

describe('EvalSettings expectation editor and failure reasons', () => {
  const fullCase: evalApi.EvalCase = {
    id: 'ec_full',
    source_run_id: 'run_full',
    status: 'draft',
    protocol: 'V2',
    input: 'edit me',
    required_reads: ['read_a'],
    consequential: ['write_b'],
    expect_consequential_any: ['write_c'],
    chain: [{ after: 'read_a', before: 'write_b' }],
    forbid_tools: ['danger'],
    arg_contains: [{ tool: 'write_b', substr: 'x' }],
    expect_state: [{ fields: { status: { op: 'eq', value: 'open' } } }],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }

  async function renderWithCases(items: evalApi.EvalCase[]) {
    mockSummary()
    vi.spyOn(evalApi, 'queryEvalRuns').mockResolvedValue({ items: [] })
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({ items })
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

  it('edits all expectation fields and status', async () => {
    const patchSpy = vi.spyOn(evalApi, 'patchEvalCase').mockResolvedValue(fullCase)
    await renderWithCases([fullCase])
    const editBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('编辑期望'),
    ) as HTMLButtonElement
    await act(async () => {
      editBtn.click()
      await Promise.resolve()
    })
    const stateArea = [...host.querySelectorAll('textarea')].find((t) =>
      t.value.includes('"status"'),
    ) as HTMLTextAreaElement
    expect(stateArea).toBeTruthy()
    expect(stateArea.value).toContain('open')

    const chainInput = [...host.querySelectorAll('input')].find(
      (i) => (i as HTMLInputElement).value === 'read_a',
    ) as HTMLInputElement
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    await act(async () => {
      setter?.call(chainInput, 'read_c')
      chainInput.dispatchEvent(new Event('input', { bubbles: true }))
    })
    const statusSelect = [...host.querySelectorAll('select')].find(
      (s) => (s as HTMLSelectElement).value === 'draft',
    ) as HTMLSelectElement
    const selectSetter = Object.getOwnPropertyDescriptor(
      HTMLSelectElement.prototype,
      'value',
    )?.set
    await act(async () => {
      selectSetter?.call(statusSelect, 'confirmed')
      statusSelect.dispatchEvent(new Event('change', { bubbles: true }))
      const areaSetter = Object.getOwnPropertyDescriptor(
        HTMLTextAreaElement.prototype,
        'value',
      )?.set
      areaSetter?.call(
        stateArea,
        '[{"fields":{"status":{"op":"eq","value":"closed"}}}]',
      )
      stateArea.dispatchEvent(new Event('input', { bubbles: true }))
    })
    const saveBtn = [...host.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === '保存',
    ) as HTMLButtonElement
    await act(async () => {
      saveBtn.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(patchSpy).toHaveBeenCalledWith(
      'ec_full',
      expect.objectContaining({
        protocol: 'V2',
        required_reads: ['read_a'],
        consequential: ['write_b'],
        expect_consequential_any: ['write_c'],
        chain: [{ after: 'read_c', before: 'write_b' }],
        forbid_tools: ['danger'],
        arg_contains: [{ tool: 'write_b', substr: 'x' }],
        expect_state: [{ fields: { status: { op: 'eq', value: 'closed' } } }],
        status: 'confirmed',
      }),
    )
  })

  it('shows readable failure reasons and compares actual calls', async () => {
    const failed: evalApi.EvalCase = {
      ...fullCase,
      id: 'ec_fail',
      status: 'confirmed',
      last_score_pass: false,
      last_score_codes: ['BSR', 'FORBID'],
      last_score_notes: 'missing_reads:list_projects; forbid:delete_project',
      last_scored_at: '2026-02-01T12:00:00Z',
    }
    const eventsSpy = vi.spyOn(runsApi, 'listEvents').mockResolvedValue([
      { type: 'llm.tool_call', timestamp: '2026-02-01T12:00:00Z', data: { name: 'search' } },
    ])
    await renderWithCases([failed])
    const reasons = host.querySelector('[data-testid="eval-reasons-ec_fail"]')
    expect(reasons).toBeTruthy()
    expect(reasons?.textContent).toContain('缺少必要读取')
    expect(reasons?.textContent).toContain('list_projects')
    expect(reasons?.textContent).toContain('调用了禁止工具')
    expect(reasons?.textContent).toContain('delete_project')

    const compareBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('失败对比'),
    ) as HTMLButtonElement
    await act(async () => {
      compareBtn.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(eventsSpy).toHaveBeenCalledWith('run_full')
    const tools = host.querySelector('[data-testid="eval-compare-tools"]')
    expect(tools?.textContent).toContain('search')
    expect(host.textContent).toContain('实际调用')
  })
})

describe('EvalSettings trend and batch actions', () => {
  const baseCase: evalApi.EvalCase = {
    id: 'ec_trend',
    source_run_id: 'run_trend',
    status: 'confirmed',
    protocol: 'V1',
    input: 'trend case',
    last_score_pass: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }

  async function renderWithItems(items: evalApi.EvalCase[]) {
    mockSummary()
    vi.spyOn(evalApi, 'queryEvalRuns').mockResolvedValue({ items: [] })
    vi.spyOn(evalApi, 'listEvalCases').mockResolvedValue({ items })
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

  it('loads score history and renders a trend', async () => {
    const scoresSpy = vi.spyOn(evalApi, 'listEvalScores').mockResolvedValue({
      items: [
        { scored_at: '2026-02-02T12:00:00Z', pass: true },
        { scored_at: '2026-02-01T12:00:00Z', pass: false, codes: ['BSR'] },
      ],
    })
    await renderWithItems([baseCase])
    const trendBtn = [...host.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === '趋势',
    ) as HTMLButtonElement
    await act(async () => {
      trendBtn.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(scoresSpy).toHaveBeenCalledWith('ec_trend', { days: undefined, limit: 100 })
    const trend = host.querySelector('[data-testid="eval-trend-ec_trend"]')
    expect(trend).toBeTruthy()
    expect(trend?.textContent).toContain('最近 2 次：通过 1')
    expect(trend?.querySelectorAll('.eval-trend-dot')).toHaveLength(2)
  })

  it('narrows the score trend to a day range', async () => {
    const scoresSpy = vi
      .spyOn(evalApi, 'listEvalScores')
      .mockResolvedValueOnce({
        items: [
          { scored_at: '2026-02-02T12:00:00Z', pass: true },
          { scored_at: '2026-02-01T12:00:00Z', pass: false },
        ],
        history_cap: 500,
      })
      .mockResolvedValueOnce({
        items: [{ scored_at: '2026-02-02T12:00:00Z', pass: true }],
        history_cap: 500,
      })
    await renderWithItems([baseCase])
    const trendBtn = [...host.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === '趋势',
    ) as HTMLButtonElement
    await act(async () => {
      trendBtn.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    const trend = host.querySelector('[data-testid="eval-trend-ec_trend"]')
    expect(trend?.textContent).toContain('每个用例保留最近 500 次评分')

    const rangeSelect = host.querySelector(
      '[data-testid="eval-trend-range-ec_trend"]',
    ) as HTMLSelectElement
    expect(rangeSelect).toBeTruthy()
    const selectSetter = Object.getOwnPropertyDescriptor(
      HTMLSelectElement.prototype,
      'value',
    )?.set
    await act(async () => {
      selectSetter?.call(rangeSelect, '30')
      rangeSelect.dispatchEvent(new Event('change', { bubbles: true }))
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(scoresSpy).toHaveBeenLastCalledWith('ec_trend', { days: 30, limit: 100 })
    expect(
      host.querySelector('[data-testid="eval-trend-ec_trend"]')?.textContent,
    ).toContain('最近 1 次：通过 1')
  })

  it('confirms selected drafts in one action', async () => {
    const draft: evalApi.EvalCase = {
      ...baseCase,
      id: 'ec_draft_batch',
      status: 'draft',
      last_score_pass: undefined,
    }
    const patchSpy = vi.spyOn(evalApi, 'patchEvalCase').mockResolvedValue(draft)
    await renderWithItems([baseCase, draft])
    const boxes = host.querySelectorAll<HTMLInputElement>('.eval-select input[type="checkbox"]')
    expect(boxes.length).toBe(2)
    await act(async () => {
      boxes[0].click()
      boxes[1].click()
    })
    const confirmBtn = [...host.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === '确认草稿',
    ) as HTMLButtonElement
    expect(confirmBtn.disabled).toBe(false)
    await act(async () => {
      confirmBtn.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(patchSpy).toHaveBeenCalledTimes(1)
    expect(patchSpy).toHaveBeenCalledWith('ec_draft_batch', { status: 'confirmed' })
  })
})
