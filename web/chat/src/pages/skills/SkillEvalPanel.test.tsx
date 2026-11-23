// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from '../../api'
import * as skillsApi from '../../api/skills'
import type { SkillSummary } from '../../api/skills'
import { GateContext } from '../../gateContext'
import { LocaleProvider } from '../../locale/LocaleContext'
import { SKILLS } from '../../strings'
import { SkillsSettings } from '../SkillsSettings'
import { SkillEvalPanel } from './SkillEvalPanel'

const sampleSkill: SkillSummary = {
  id: 'user-skill',
  name: '用户技能',
  description: '',
  tools: [],
  source: 'user',
  eval_case_count: 2,
}

const evalDoc = {
  harness: 'evidence-eval',
  cases: [
    { id: 'case-a', protocol: 'V0', input: '列出工单' },
    { id: 'case-b', protocol: 'V1', input: '更新状态' },
  ],
}

async function renderSkills(role: 'admin' | 'operator' = 'admin') {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  await act(async () => {
    root.render(
      createElement(
        LocaleProvider,
        null,
        createElement(
          GateContext.Provider,
          { value: { role, gateEnabled: true, operatorId: role } },
          createElement(SkillsSettings),
        ),
      ),
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

async function renderPanel(
  role: 'admin' | 'operator',
  skill: SkillSummary = sampleSkill,
) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  await act(async () => {
    root.render(
      createElement(
        LocaleProvider,
        null,
        createElement(SkillEvalPanel, {
          skill,
          agentId: 'ticket-agent',
          role,
        }),
      ),
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

function evalButton(host: ParentNode): HTMLButtonElement {
  const btn = [...host.querySelectorAll('button')].find((b) => b.textContent?.includes(SKILLS.evalOpen))
  if (!btn) throw new Error('eval open button not found')
  return btn as HTMLButtonElement
}

describe('SkillEvalPanel', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows eval count and opens panel', async () => {
    vi.spyOn(api, 'getUIConfig').mockResolvedValue({
      agent_id: 'ticket-agent',
      gate_enabled: true,
      supports_vision: false,
    })
    vi.spyOn(api, 'listSkills').mockResolvedValue({
      skills: [sampleSkill],
    })
    vi.spyOn(api, 'getAgent').mockResolvedValue({ id: 'ticket-agent', system: '', skills: [] })
    vi.spyOn(skillsApi, 'getSkillEval').mockResolvedValue(evalDoc)

    const { host, root } = await renderSkills('admin')

    expect(host.textContent).toContain(SKILLS.evalCount(2))

    await act(async () => {
      evalButton(host).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(skillsApi.getSkillEval).toHaveBeenCalledWith('user-skill')
    expect(host.textContent).toContain('列出工单')

    root.unmount()
    host.remove()
  })

  it('admin save calls putSkillEval', async () => {
    vi.spyOn(skillsApi, 'getSkillEval').mockResolvedValue({
      harness: 'evidence-eval',
      cases: [{ id: 'only', protocol: 'V0', input: 'hi' }],
    })
    const putSpy = vi.spyOn(skillsApi, 'putSkillEval').mockResolvedValue({
      harness: 'evidence-eval',
      cases: [{ id: 'only', protocol: 'V0', input: 'hi there' }],
    })

    const { host, root } = await renderPanel('admin')
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    const idInput = host.querySelector('input.settings-input') as HTMLInputElement
    expect(idInput).toBeTruthy()
    await act(async () => {
      const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
        window.HTMLInputElement.prototype,
        'value',
      )!.set!
      nativeInputValueSetter.call(idInput, 'renamed')
      idInput.dispatchEvent(new Event('input', { bubbles: true }))
    })

    const saveBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(SKILLS.evalSave),
    )
    expect(saveBtn).toBeTruthy()
    await act(async () => {
      saveBtn!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(putSpy).toHaveBeenCalledWith('user-skill', {
      harness: 'evidence-eval',
      cases: [{ id: 'renamed', protocol: 'V0', input: 'hi' }],
    })

    root.unmount()
    host.remove()
  })

  it('preserves required_reads when editing input before save', async () => {
    vi.spyOn(skillsApi, 'getSkillEval').mockResolvedValue({
      harness: 'evidence-eval',
      cases: [
        {
          id: 'c1',
          protocol: 'V0',
          input: 'old prompt',
          required_reads: ['list_tickets'],
        },
      ],
    })
    const putSpy = vi.spyOn(skillsApi, 'putSkillEval').mockImplementation(async (_id, doc) => doc)

    const { host, root } = await renderPanel('admin')
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    const textarea = host.querySelector('textarea') as HTMLTextAreaElement
    expect(textarea).toBeTruthy()
    await act(async () => {
      const nativeSetter = Object.getOwnPropertyDescriptor(
        window.HTMLTextAreaElement.prototype,
        'value',
      )!.set!
      nativeSetter.call(textarea, 'new prompt')
      textarea.dispatchEvent(new Event('input', { bubbles: true }))
    })

    const saveBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(SKILLS.evalSave),
    )
    await act(async () => {
      saveBtn!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(putSpy).toHaveBeenCalledWith('user-skill', {
      harness: 'evidence-eval',
      cases: [
        {
          id: 'c1',
          protocol: 'V0',
          input: 'new prompt',
          required_reads: ['list_tickets'],
        },
      ],
    })

    root.unmount()
    host.remove()
  })

  it('blocks save when input is empty', async () => {
    vi.spyOn(skillsApi, 'getSkillEval').mockResolvedValue({
      harness: 'evidence-eval',
      cases: [{ id: 'only', protocol: 'V0', input: 'ok' }],
    })
    const putSpy = vi.spyOn(skillsApi, 'putSkillEval')

    const { host, root } = await renderPanel('admin')
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    await act(async () => {
      ;[...host.querySelectorAll('button')].find((b) => b.textContent?.includes(SKILLS.evalAddCase))!.click()
      await new Promise((r) => setTimeout(r, 0))
    })

    await act(async () => {
      ;[...host.querySelectorAll('button')].find((b) => b.textContent?.includes(SKILLS.evalSave))!.click()
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(putSpy).not.toHaveBeenCalled()
    expect(host.textContent).toContain(SKILLS.evalValidationBeforeSave)

    root.unmount()
    host.remove()
  })

  it('run confirms then posts runSkillEval', async () => {
    vi.spyOn(skillsApi, 'getSkillEval').mockResolvedValue(evalDoc)
    const runSpy = vi.spyOn(skillsApi, 'runSkillEval').mockResolvedValue({
      results: [{ id: 'case-a', pass: true, codes: [], notes: '', run_id: 'run-1' }],
    })
    const confirmSpy = vi.spyOn(window, 'confirm')

    const { host, root } = await renderPanel('admin')
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    const runAllBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(SKILLS.evalRunAll),
    )
    expect(runAllBtn).toBeTruthy()
    await act(async () => {
      runAllBtn!.click()
    })

    expect(confirmSpy).not.toHaveBeenCalled()
    expect(host.textContent).toContain(SKILLS.evalConfirmRunBody)
    expect(host.textContent).toMatch(/token/i)

    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(runSpy).toHaveBeenCalledWith('user-skill', { agent_id: 'ticket-agent', case_ids: undefined })

    root.unmount()
    host.remove()
  })

  it('builtin eval is read-only but admin can run after confirm', async () => {
    const builtinSkill: SkillSummary = {
      id: 'builtin-skill',
      name: '内置',
      description: '',
      tools: [],
      source: 'builtin',
      eval_case_count: 1,
    }
    vi.spyOn(skillsApi, 'getSkillEval').mockResolvedValue({
      harness: 'evidence-eval',
      cases: [{ id: 'bcase', protocol: 'V0', input: 'hello builtin' }],
    })
    const runSpy = vi.spyOn(skillsApi, 'runSkillEval').mockResolvedValue({
      results: [{ id: 'bcase', pass: true, codes: [], notes: '', run_id: 'run-b' }],
    })
    const putSpy = vi.spyOn(skillsApi, 'putSkillEval')

    const { host, root } = await renderPanel('admin', builtinSkill)
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(host.textContent).not.toContain(SKILLS.evalSave)
    expect(host.textContent).toContain(SKILLS.evalReadonlyBuiltin)
    expect(host.querySelector('textarea')).toBeNull()

    const runAllBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(SKILLS.evalRunAll),
    )
    await act(async () => {
      runAllBtn!.click()
    })
    expect(host.textContent).toContain(SKILLS.evalConfirmRunBody)

    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(runSpy).toHaveBeenCalledWith('builtin-skill', {
      agent_id: 'ticket-agent',
      case_ids: undefined,
    })
    expect(putSpy).not.toHaveBeenCalled()

    root.unmount()
    host.remove()
  })

  it('operator sees cases but no run or save controls', async () => {
    vi.spyOn(skillsApi, 'getSkillEval').mockResolvedValue(evalDoc)
    const runSpy = vi.spyOn(skillsApi, 'runSkillEval')
    const putSpy = vi.spyOn(skillsApi, 'putSkillEval')

    const { host, root } = await renderPanel('operator')
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(host.textContent).toContain('case-a')
    expect(host.textContent).not.toContain(SKILLS.evalSave)
    expect(host.textContent).not.toContain(SKILLS.evalRunAll)
    expect(runSpy).not.toHaveBeenCalled()
    expect(putSpy).not.toHaveBeenCalled()

    root.unmount()
    host.remove()
  })
})
