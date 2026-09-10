// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { Composer } from './Composer'
import type { SkillSummary } from '../api'

const SKILLS: SkillSummary[] = [
  { id: 'data-analytics', name: '数据分析', description: '分析数据', tools: [], source: 'builtin' },
  { id: 'ticket-triage', name: '工单分诊', description: '工单分诊', tools: [], source: 'builtin' },
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

describe('Composer skill completion placement (regression: off-screen popup)', () => {
  it('opens the completion popup inside .composer-box for "@" trigger', () => {
    render(<Composer onSend={() => true} skills={SKILLS} />)
    typeText('@')
    const box = host.querySelector('.composer-box')
    expect(popup()).not.toBeNull()
    // The popup is absolutely positioned relative to .composer-box; it must be
    // a descendant of it or it anchors to the page and renders off-screen.
    expect(box?.contains(popup())).toBe(true)
    expect(popup()?.querySelectorAll('.composer-complete-item')).toHaveLength(2)
  })

  it('opens the completion popup inside .composer-box for "/" trigger', () => {
    render(<Composer onSend={() => true} skills={SKILLS} />)
    typeText('/')
    const box = host.querySelector('.composer-box')
    expect(popup()).not.toBeNull()
    expect(box?.contains(popup())).toBe(true)
  })

  it('does not render a popup when no skills are provided', () => {
    render(<Composer onSend={() => true} skills={[]} />)
    typeText('@')
    expect(popup()).toBeNull()
  })
})
