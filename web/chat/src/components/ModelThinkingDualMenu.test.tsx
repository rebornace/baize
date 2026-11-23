// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ModelProfile } from '../api'
import { LocaleProvider } from '../locale/LocaleContext'
import { setPack } from '../locale/pack'
import { LOCALE_STORAGE_KEY } from '../locale/types'
import { zhPack } from '../locales/zh'
import { ModelThinkingDualMenu } from './ModelThinkingDualMenu'

const profiles: ModelProfile[] = [
  {
    id: 'm1',
    name: 'Fast',
    provider: 'openai',
    base_url: 'https://example.com/v1',
    model: 'fast',
    disable_thinking: false,
    thinking_level: 'medium',
    thinking_dialect: 'auto',
    supports_vision: false,
    context_tokens: 128000,
    auto_tier: 'standard',
  },
]

let host: HTMLDivElement
let root: Root

beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
    true
  localStorage.clear()
  localStorage.setItem(LOCALE_STORAGE_KEY, 'zh-CN')
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
})

describe('ModelThinkingDualMenu', () => {
  it('renders a Cursor-style ghost trigger with model name', () => {
    act(() => {
      root.render(
        <LocaleProvider>
          <ModelThinkingDualMenu
            profiles={profiles}
            modelId="m1"
            thinkingLevel=""
            onChooseModel={() => {}}
            onChooseThinking={() => {}}
          />
        </LocaleProvider>,
      )
    })
    expect(host.querySelector('[data-testid="model-thinking-dual"]')).toBeTruthy()
    expect(host.querySelector('.model-picker-trigger')).toBeTruthy()
    expect(host.querySelector('[data-testid="model-chip"]')?.textContent).toContain('Fast')
    expect(host.querySelector('.model-picker-menu')).toBeNull()
  })

  it('opens one panel with model section and thinking section', () => {
    const onChooseThinking = vi.fn()
    act(() => {
      root.render(
        <LocaleProvider>
          <ModelThinkingDualMenu
            profiles={profiles}
            modelId="m1"
            thinkingLevel=""
            onChooseModel={() => {}}
            onChooseThinking={onChooseThinking}
          />
        </LocaleProvider>,
      )
    })
    act(() => {
      host.querySelector<HTMLButtonElement>('.model-picker-trigger')?.click()
    })
    const menu = host.querySelector('.model-picker-menu')
    expect(menu).toBeTruthy()
    expect(menu?.textContent).toContain('选择模型')
    expect(menu?.textContent).toContain('思考级别')
    expect(menu?.textContent).toContain('Fast')
    const thinkingItem = Array.from(menu!.querySelectorAll('[role="menuitemradio"]')).find((el) =>
      el.textContent?.includes('关'),
    ) as HTMLButtonElement
    act(() => {
      thinkingItem.click()
    })
    expect(onChooseThinking).toHaveBeenCalledWith('off')
  })
})
