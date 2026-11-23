// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as connectors from '../../api/connectors'
import * as skillsApi from '../../api/skills'
import { SKILLS } from '../../strings'
import { SkillWorkflowEditor } from './SkillWorkflowEditor'

const skill: skillsApi.SkillSummary = {
  id: 'demo',
  name: 'demo',
  description: '',
  tools: ['ta'],
  source: 'user',
}

async function renderEditor(readOnly = false) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  await act(async () => {
    root.render(
      createElement(SkillWorkflowEditor, {
        skill,
        readOnly,
        onChanged: () => undefined,
        onClose: () => undefined,
      }),
    )
    await Promise.resolve()
    await Promise.resolve()
  })
  return { host, root }
}

function button(host: HTMLElement, text: string): HTMLButtonElement | undefined {
  return Array.from(host.querySelectorAll('button')).find((b) => b.textContent?.includes(text)) as
    | HTMLButtonElement
    | undefined
}

describe('SkillWorkflowEditor', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('creates and edits a workflow draft', async () => {
    vi.spyOn(skillsApi, 'getSkillWorkflow').mockResolvedValue({
      skill_id: 'demo',
      editable: true,
      can_create: true,
      source: 'user',
    })
    vi.spyOn(connectors, 'listTools').mockResolvedValue([
      { name: 'ta', connector_id: 'c', enabled: true },
    ])
    const { host } = await renderEditor()
    expect(host.textContent).toContain(SKILLS.workflowEmpty)
    const create = button(host, SKILLS.workflowCreate)
    expect(create).toBeTruthy()
    await act(async () => {
      create?.click()
    })
    const add = button(host, SKILLS.workflowAddStep)
    expect(add).toBeTruthy()
    await act(async () => {
      add?.click()
    })
    expect(host.textContent).toContain(SKILLS.workflowStep(1))
    expect(Array.from(host.querySelectorAll('input')).some((i) => i.value === 'step1')).toBe(true)
  })

  it('edits and saves retry and timeout', async () => {
    vi.spyOn(skillsApi, 'getSkillWorkflow').mockResolvedValue({
      skill_id: 'demo',
      editable: true,
      can_create: true,
      source: 'user',
      workflow: {
        name: 'demo',
        steps: [{ id: 'a', tool: 'ta', args: {}, retry: 2, timeout_ms: 20000 }],
      },
    })
    vi.spyOn(connectors, 'listTools').mockResolvedValue([
      { name: 'ta', connector_id: 'c', enabled: true },
    ])
    const put = vi.spyOn(skillsApi, 'putSkillWorkflow').mockResolvedValue({
      skill_id: 'demo',
      editable: true,
      can_create: false,
      source: 'user',
    })
    const { host } = await renderEditor()
    const numbers = host.querySelectorAll<HTMLInputElement>('input[type="number"]')
    const checkboxes = host.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')
    expect(numbers[0]?.value).toBe('2')
    expect(numbers[1]?.value).toBe('20')
    expect(checkboxes.length).toBe(3)
    const setValue = (input: HTMLInputElement, value: string) => {
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
      setter?.call(input, value)
      input.dispatchEvent(new Event('input', { bubbles: true }))
    }
    await act(async () => {
      setValue(numbers[0] as HTMLInputElement, '4')
      setValue(numbers[1] as HTMLInputElement, '30')
      checkboxes[1]?.click()
    })
    await act(async () => {
      button(host, SKILLS.workflowSave)?.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(put).toHaveBeenCalledWith(
      'demo',
      expect.objectContaining({
        steps: [expect.objectContaining({ retry: 4, timeout_ms: 30000, continue_on_error: true })],
      }),
    )
  })

  it('switches to raw YAML and previews rendered args', async () => {
    vi.spyOn(skillsApi, 'getSkillWorkflow').mockResolvedValue({
      skill_id: 'demo',
      editable: true,
      can_create: true,
      source: 'user',
      workflow: { name: 'demo', steps: [{ id: 'a', tool: 'ta', args: {} }] },
      yaml: 'name: demo\nsteps:\n  - id: a\n    tool: ta\n',
    })
    vi.spyOn(connectors, 'listTools').mockResolvedValue([
      { name: 'ta', connector_id: 'c', enabled: true },
    ])
    const previewSpy = vi.spyOn(skillsApi, 'previewSkillWorkflow').mockResolvedValue({
      steps: [{ id: 'a', tool: 'ta', status: 'rendered', args: { q: 'open' } }],
    })
    const { host } = await renderEditor()
    const yamlButton = button(host, SKILLS.workflowYAML)
    await act(async () => {
      yamlButton?.click()
    })
    const area = host.querySelector('textarea') as HTMLTextAreaElement | null
    expect(area?.value).toContain('name: demo')
    const previewButton = button(host, SKILLS.workflowPreviewRun)
    await act(async () => {
      previewButton?.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(previewSpy).toHaveBeenCalled()
    expect(host.textContent).toContain('rendered')
    expect(host.textContent).toContain('"q":"open"')
  })

  it('shows a read-only workflow without create controls', async () => {
    vi.spyOn(skillsApi, 'getSkillWorkflow').mockResolvedValue({
      skill_id: 'demo',
      editable: false,
      can_create: false,
      source: 'builtin',
      workflow: { name: 'demo', steps: [{ id: 'a', tool: 'ta', args: {} }] },
    })
    vi.spyOn(connectors, 'listTools').mockResolvedValue([])
    const { host } = await renderEditor(true)
    expect(host.textContent).toContain(SKILLS.workflowReadOnly)
    expect(button(host, SKILLS.workflowSave)).toBeFalsy()
    expect(host.textContent).toContain('"id": "a"')
  })
})
