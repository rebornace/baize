// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { LocaleProvider } from '../locale/LocaleContext'
import { setPack } from '../locale/pack'
import { LOCALE_STORAGE_KEY } from '../locale/types'
import { zhPack } from '../locales/zh'
import { CHAT } from '../strings'
import { ThinkingBlock } from './ThinkingBlock'
import type { ThinkingChatBlock } from './ThinkingBlock'

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  localStorage.clear()
  localStorage.setItem(LOCALE_STORAGE_KEY, 'zh-CN')
  setPack(zhPack)
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => {
  host.remove()
  localStorage.clear()
  setPack(zhPack)
})

function block(partial: Partial<ThinkingChatBlock> = {}): ThinkingChatBlock {
  return {
    kind: 'thinking',
    turn: 0,
    text: '这是一段很长的推理过程，用来确认折叠时不会渲染正文。',
    status: 'streaming',
    collapsed: false,
    ...partial,
  }
}

function render(props: { block: ThinkingChatBlock; readOnly?: boolean }) {
  act(() => {
    createRoot(host).render(
      <LocaleProvider>
        <ThinkingBlock {...props} />
      </LocaleProvider>,
    )
  })
}

describe('ThinkingBlock', () => {
  it('stays one line while streaming and does not render thinking text', () => {
    render({ block: block() })
    const el = host.querySelector('[data-testid="thinking-block"]')
    expect(el?.textContent).toContain(CHAT.thinking)
    expect(el?.textContent).not.toContain('很长的推理')
    expect(el?.querySelector('.thinking-block-body')).toBeNull()
    expect(el?.classList.contains('thinking-block-open')).toBe(false)
  })

  it('stays collapsed after thinking finishes until the user expands', () => {
    render({
      block: block({ status: 'done', collapsed: true, text: '推理完成内容' }),
    })
    const el = host.querySelector('[data-testid="thinking-block"]')
    expect(el?.textContent).toContain(CHAT.thoughtDone)
    expect(el?.textContent).not.toContain('推理完成内容')
    const toggle = host.querySelector('.thinking-block-toggle') as HTMLButtonElement
    expect(toggle.disabled).toBe(false)
    act(() => {
      toggle.click()
    })
    expect(host.querySelector('.thinking-block-body')?.textContent).toContain('推理完成内容')
  })
})
