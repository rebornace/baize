// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MessageSearch } from './MessageSearch'

let host: HTMLDivElement
let root: Root

beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
})

afterEach(() => {
  act(() => {
    root?.unmount()
  })
  host.remove()
  vi.unstubAllGlobals()
})

function render(el: ReactNode) {
  act(() => {
    root = createRoot(host)
    root.render(el)
  })
}

async function flush() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
}

async function typeQuery(value: string) {
  const input = host.querySelector('.message-search-input') as HTMLInputElement
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
  act(() => {
    setter?.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
  await flush()
}

describe('MessageSearch', () => {
  it('searches all conversations and opens a hit', async () => {
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
      <MessageSearch
        open
        conversationId="c-current"
        debounceMs={0}
        onClose={() => {}}
        onOpenConversation={onOpen}
      />,
    )
    expect(host.textContent).toContain('输入关键词开始搜索。')
    await typeQuery('ORD-4242')
    expect(host.textContent).toContain('排查 VPN')
    expect(host.textContent).toContain('ORD-4242 的物流')
    expect(host.textContent).toContain('你')
    const lastCall = fetchMock.mock.calls.at(-1) as unknown[] | undefined
    const url = String(lastCall?.[0])
    const params = new URLSearchParams(url.split('?')[1])
    expect(url.startsWith('/v0/conversations/search?')).toBe(true)
    expect(params.get('q')).toBe('ORD-4242')
    expect(params.get('limit')).toBe('20')
    expect(params.get('role')).toBeNull()
    expect(params.get('days')).toBeNull()
    expect(params.get('conversation_id')).toBeNull()
    const hit = host.querySelector('.message-search-hit') as HTMLButtonElement
    expect(hit.getAttribute('data-message-id')).toBe('m1')
    act(() => {
      hit.click()
    })
    expect(onOpen).toHaveBeenCalledWith('c1', 'm1')
  })

  it('shows the empty state and error state', async () => {
    const fetchMock = vi.fn(async () => ({ ok: true, json: async () => ({ hits: [] }) }))
    vi.stubGlobal('fetch', fetchMock)
    render(
      <MessageSearch
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
    const fetchMock = vi.fn(async () => ({
      ok: true,
      json: async () => ({ hits: [] }),
    }))
    vi.stubGlobal('fetch', fetchMock)
    render(
      <MessageSearch
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
})
