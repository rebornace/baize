// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GateContext } from '../../gateContext'
import { LocaleProvider } from '../../locale/LocaleContext'
import { setPack } from '../../locale/pack'
import { zhPack } from '../../locales/zh'
import { UnifiedSearch } from './UnifiedSearch'

let host: HTMLDivElement
let root: Root

beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  localStorage.clear()
  localStorage.setItem('baize.locale', 'zh-CN')
  setPack(zhPack)
  host = document.createElement('div')
  document.body.appendChild(host)
})

afterEach(() => {
  act(() => {
    root?.unmount()
  })
  host.remove()
  vi.unstubAllGlobals()
  localStorage.clear()
  setPack(zhPack)
})

function render(el: ReactNode) {
  act(() => {
    root = createRoot(host)
    root.render(
      <LocaleProvider>
        <MemoryRouter>
          <GateContext.Provider value={{ role: 'admin', gateEnabled: false, operatorId: 'op' }}>
            {el}
          </GateContext.Provider>
        </MemoryRouter>
      </LocaleProvider>,
    )
  })
}

async function flush() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
}

async function typeQuery(value: string) {
  const input = host.querySelector('[data-testid="unified-search-input"]') as HTMLInputElement
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
  act(() => {
    setter?.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
  await flush()
}

describe('UnifiedSearch', () => {
  it('searches messages by default and opens a hit', async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      json: async () => ({
        hits: [
          {
            conversation_id: 'c1',
            conversation_title: '排查 VPN',
            message_id: 'm1',
            role: 'user',
            snippet: 'ORD-4242 的物流',
          },
        ],
      }),
    }))
    vi.stubGlobal('fetch', fetchMock)
    const onOpen = vi.fn()
    render(
      <UnifiedSearch
        open
        conversationId="c-current"
        debounceMs={0}
        onClose={() => {}}
        onOpenConversation={onOpen}
      />,
    )
    expect(host.textContent).toContain('消息')
    expect(host.textContent).toContain('功能与设置')
    expect(host.textContent).toContain('输入关键词开始搜索。')
    await typeQuery('ORD-4242')
    expect(host.textContent).toContain('排查 VPN')
    expect(host.textContent).toContain('ORD-4242 的物流')
    const hit = host.querySelector('.message-search-hit') as HTMLButtonElement
    act(() => {
      hit.click()
    })
    expect(onOpen).toHaveBeenCalledWith('c1', 'm1')
  })

  it('shows the empty and error states', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: true, json: async () => ({ hits: [] }) })),
    )
    render(
      <UnifiedSearch
        open
        conversationId="c-current"
        debounceMs={0}
        onClose={() => {}}
        onOpenConversation={() => {}}
      />,
    )
    await typeQuery('nope')
    expect(host.textContent).toContain('没有匹配的消息。')

    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: false, status: 500, statusText: 'boom', json: async () => ({}) })),
    )
    await typeQuery('fail')
    expect(host.textContent).toContain('搜索失败，请重试。')
  })

  it('sends role, time and current-conversation filters', async () => {
    const fetchMock = vi.fn(async () => ({ ok: true, json: async () => ({ hits: [] }) }))
    vi.stubGlobal('fetch', fetchMock)
    render(
      <UnifiedSearch
        open
        conversationId="c9"
        debounceMs={0}
        onClose={() => {}}
        onOpenConversation={() => {}}
      />,
    )
    const selects = host.querySelectorAll<HTMLSelectElement>('.message-search-filters select')
    act(() => {
      selects[0].value = 'assistant'
      selects[0].dispatchEvent(new Event('change', { bubbles: true }))
    })
    act(() => {
      selects[1].value = '7'
      selects[1].dispatchEvent(new Event('change', { bubbles: true }))
    })
    const scope = host.querySelector<HTMLInputElement>('.message-search-scope input')
    act(() => {
      scope?.click()
    })
    await typeQuery('PIN')

    const lastCall = fetchMock.mock.calls.at(-1) as unknown[] | undefined
    const url = String(lastCall?.[0])
    const params = new URLSearchParams(url.split('?')[1])
    expect(params.get('role')).toBe('assistant')
    expect(params.get('days')).toBe('7')
    expect(params.get('conversation_id')).toBe('c9')
  })

  it('switches to the features tab and filters settings entries', async () => {
    render(
      <UnifiedSearch
        open
        conversationId="c1"
        onClose={() => {}}
        onOpenConversation={() => {}}
      />,
    )
    const tabs = host.querySelectorAll<HTMLButtonElement>('[role="tab"]')
    act(() => {
      tabs[1].click()
    })
    await typeQuery('模型')
    expect(host.textContent).toContain('模型')
    expect(host.textContent).toContain('页面')
  })
})
