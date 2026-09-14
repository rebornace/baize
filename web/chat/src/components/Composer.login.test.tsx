// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Composer } from './Composer'
import type { LoginEntry, SkillSummary } from '../api'
import { LOGIN_AT } from '../strings'

const SKILLS: SkillSummary[] = [
  { id: 'data-analytics', name: '数据分析', description: '分析数据', tools: [], source: 'builtin' },
  { id: 'ticket-triage', name: '工单分诊', description: '工单分诊', tools: [], source: 'builtin' },
]

function entry(partial: Partial<LoginEntry> & Pick<LoginEntry, 'id' | 'title'>): LoginEntry {
  return {
    connector_id: 'crm',
    connector_title: 'CRM',
    connector_type: 'openapi',
    tool_name: 'login',
    logged_in: false,
    required: [],
    ...partial,
  }
}

const LOGINS: LoginEntry[] = [
  entry({ id: 'crm/login', title: '登录 · CRM / login', logged_in: true, required: [] }),
  entry({
    id: 'crm/auth',
    title: '登录 · CRM / auth',
    tool_name: 'auth',
    required: ['username', 'password'],
  }),
]

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => host.remove())

function render(el: ReactNode) {
  act(() => {
    createRoot(host).render(el)
  })
}

function textarea(): HTMLTextAreaElement {
  return host.querySelector('.composer-box textarea') as HTMLTextAreaElement
}

function typeText(value: string) {
  const ta = textarea()
  const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value')!.set!
  act(() => {
    setter.call(ta, value)
    ta.setSelectionRange(value.length, value.length)
    ta.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

function popup(): HTMLElement | null {
  return host.querySelector('.composer-complete')
}

function clickItem(label: string) {
  const items = Array.from(host.querySelectorAll('.composer-complete-item'))
  const target = items.find((el) => el.textContent?.includes(label))
  expect(target).toBeTruthy()
  act(() => {
    target!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true }))
  })
}

describe('Composer login completion', () => {
  it('shows login section above skills for @ with logged-in badge', () => {
    render(<Composer onSend={() => true} skills={SKILLS} loginEntries={LOGINS} />)
    typeText('@')
    const box = popup()
    expect(box).not.toBeNull()
    expect(box!.textContent).toContain(LOGIN_AT.sectionLogin)
    expect(box!.textContent).toContain(LOGIN_AT.sectionSkills)
    expect(box!.textContent).toContain('登录 · CRM / login')
    expect(box!.textContent).toContain(LOGIN_AT.loggedInBadge)
    expect(box!.textContent).toContain('data-analytics')
    // 登录区在技能区之上
    expect(box!.textContent!.indexOf(LOGIN_AT.sectionLogin)).toBeLessThan(
      box!.textContent!.indexOf(LOGIN_AT.sectionSkills),
    )
  })

  it('opens the same mixed popup for /', () => {
    render(<Composer onSend={() => true} skills={SKILLS} loginEntries={LOGINS} />)
    typeText('/')
    expect(popup()?.textContent).toContain(LOGIN_AT.sectionLogin)
    expect(popup()?.textContent).toContain('登录 · CRM / login')
  })

  it('picks required:[] via onPickLogin without inserting @skill mention', () => {
    const onPickLogin = vi.fn()
    render(
      <Composer
        onSend={() => true}
        skills={SKILLS}
        loginEntries={LOGINS}
        onPickLogin={onPickLogin}
      />,
    )
    typeText('@')
    clickItem('登录 · CRM / login')
    expect(onPickLogin).toHaveBeenCalledTimes(1)
    expect(onPickLogin).toHaveBeenCalledWith(LOGINS[0])
    // 清除 @ 查询片段，不写入技能 mention
    expect(textarea().value).toBe('')
    expect(popup()).toBeNull()
  })

  it('opens LoginParamsModal for required fields and submits args', () => {
    const onPickLogin = vi.fn()
    render(
      <Composer
        onSend={() => true}
        skills={SKILLS}
        loginEntries={LOGINS}
        onPickLogin={onPickLogin}
      />,
    )
    typeText('@')
    clickItem('登录 · CRM / auth')
    expect(onPickLogin).not.toHaveBeenCalled()
    const modal = host.querySelector('[data-testid="modal-panel"]')
    expect(modal).not.toBeNull()
    expect(modal!.textContent).toContain(LOGIN_AT.paramsTitle)

    const userInput = host.querySelector('input[name="username"]') as HTMLInputElement
    const passInput = host.querySelector('input[name="password"]') as HTMLInputElement
    expect(userInput).toBeTruthy()
    expect(passInput?.type).toBe('password')

    act(() => {
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
      setter.call(userInput, 'alice')
      userInput.dispatchEvent(new Event('input', { bubbles: true }))
      setter.call(passInput, 's3cret')
      passInput.dispatchEvent(new Event('input', { bubbles: true }))
    })

    const submit = Array.from(host.querySelectorAll('button')).find(
      (b) => b.textContent === LOGIN_AT.paramsSubmit,
    )
    expect(submit).toBeTruthy()
    act(() => {
      submit!.click()
    })

    expect(onPickLogin).toHaveBeenCalledWith(LOGINS[1], {
      username: 'alice',
      password: 's3cret',
    })
  })

  it('keeps skills-only popup behavior when loginEntries omitted', () => {
    render(<Composer onSend={() => true} skills={SKILLS} />)
    typeText('@')
    expect(popup()?.querySelectorAll('.composer-complete-item')).toHaveLength(2)
    expect(popup()?.textContent).not.toContain(LOGIN_AT.sectionLogin)
  })
})
