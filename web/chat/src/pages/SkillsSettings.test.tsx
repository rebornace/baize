// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import { GateContext } from '../gateContext'
import { SKILLS } from '../strings'
import { SkillsSettings } from './SkillsSettings'

async function renderSkills(role: 'admin' | 'operator' = 'admin') {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  await act(async () => {
    root.render(
      createElement(
        GateContext.Provider,
        { value: { role, gateEnabled: true, operatorId: role } },
        createElement(SkillsSettings),
      ),
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

describe('SkillsSettings humanized shell', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('uses humanized title and ConfirmDialog on user skill delete', async () => {
    vi.spyOn(api, 'getUIConfig').mockResolvedValue({})
    vi.spyOn(api, 'listSkills').mockResolvedValue({
      skills: [
        {
          id: 'builtin-skill',
          name: '内置技能',
          description: '',
          tools: [],
          source: 'builtin',
        },
        {
          id: 'user-skill',
          name: '用户技能',
          description: '自定义说明',
          tools: ['tool_a'],
          source: 'user',
        },
      ],
    })
    vi.spyOn(api, 'getAgent').mockResolvedValue({
      id: 'ticket-agent',
      system: '',
      skills: ['builtin-skill'],
    })
    const deleteSpy = vi.spyOn(api, 'deleteSkill').mockResolvedValue()
    const confirmSpy = vi.spyOn(window, 'confirm')

    const { host, root } = await renderSkills('admin')

    expect(host.textContent).toContain(SKILLS.title)
    expect(host.textContent).not.toMatch(/\bSkills\b/)
    expect(host.textContent).not.toMatch(/默认 Agent/)
    expect(host.textContent).toContain(SKILLS.saveDefaults)
    expect(host.textContent).toContain(SKILLS.saveDefaultsHint)
    expect(host.querySelectorAll('input[type="checkbox"]').length).toBe(2)

    const deleteBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(SKILLS.confirmDeleteOk),
    )
    expect(deleteBtn).toBeTruthy()
    await act(async () => {
      deleteBtn!.click()
    })

    expect(confirmSpy).not.toHaveBeenCalled()
    expect(deleteSpy).not.toHaveBeenCalled()
    expect(host.textContent).toContain(SKILLS.confirmDeleteTitle)
    expect(host.textContent).toContain(SKILLS.confirmDeleteBody)

    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(deleteSpy).toHaveBeenCalledWith('user-skill')
    expect(host.textContent).toContain(SKILLS.toastDeleted)

    root.unmount()
    host.remove()
  })

  it('hides checkboxes and default controls for operator', async () => {
    vi.spyOn(api, 'getUIConfig').mockResolvedValue({})
    vi.spyOn(api, 'listSkills').mockResolvedValue({
      skills: [
        {
          id: '分诊',
          name: '分诊',
          description: '',
          tools: [],
          source: 'builtin',
        },
        {
          id: '自定义',
          name: '自定义',
          description: '',
          tools: [],
          source: 'user',
        },
      ],
    })
    const getAgentSpy = vi.spyOn(api, 'getAgent')

    const { host, root } = await renderSkills('operator')

    expect(host.textContent).toContain(SKILLS.title)
    expect(host.textContent).toContain('分诊')
    expect(host.textContent).toContain(SKILLS.sourceBuiltin)
    expect(host.textContent).toContain(SKILLS.sourceUser)
    expect(host.querySelector('input[type="file"]')).toBeNull()
    expect(host.textContent).not.toContain(SKILLS.saveDefaults)
    expect(host.textContent).not.toContain(SKILLS.saveDefaultsHint)
    expect(host.textContent).not.toContain(SKILLS.confirmDeleteOk)
    expect(host.textContent).not.toContain(SKILLS.defaultBadge)
    expect(getAgentSpy).not.toHaveBeenCalled()
    expect(host.querySelectorAll('input[type="checkbox"]').length).toBe(0)

    root.unmount()
    host.remove()
  })
})
