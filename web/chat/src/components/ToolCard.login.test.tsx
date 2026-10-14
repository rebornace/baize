// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatBlock } from '../foldEvents'
import { LOGIN_AT } from '../strings'
import { ToolCard } from './ToolCard'

const catalog = [
  {
    name: 'order_query',
    title: '查询订单',
    description: '按单号查询',
    connector_id: 'crm',
  },
]

const loginResult = { code: 'login_required', message: '此工具需要先登录' }

const tool = (over: Partial<Extract<ChatBlock, { kind: 'tool' }>> = {}) =>
  ({
    kind: 'tool',
    name: 'order_query',
    status: 'failed',
    runId: 'r1',
    result: loginResult,
    isError: true,
    ...over,
  }) as Extract<ChatBlock, { kind: 'tool' }>

describe('ToolCard login_required', () => {
  it('shows login message without chat-login button', () => {
    const html = renderToStaticMarkup(
      <ToolCard block={tool()} catalog={catalog} onBrowserLogin={() => {}} />,
    )
    expect(html).toContain('此工具需要先登录')
    expect(html).toContain(LOGIN_AT.preferBrowser)
    expect(html).toContain(LOGIN_AT.browserLogin)
    expect(html).not.toContain(LOGIN_AT.localHint)
  })

  it('shows cloud risk copy and browser login when provided', () => {
    const html = renderToStaticMarkup(
      <ToolCard
        block={tool()}
        catalog={catalog}
        cloudModelRisk
        onBrowserLogin={() => {}}
      />,
    )
    expect(html).toContain(LOGIN_AT.cloudRisk)
    expect(html).toContain(LOGIN_AT.browserLogin)
    expect(html).toContain('tool-card-browser-login')
  })

  it('keeps browser login on readOnly history cards', () => {
    const html = renderToStaticMarkup(
      <ToolCard
        block={tool()}
        catalog={catalog}
        readOnly
        onBrowserLogin={() => {}}
      />,
    )
    expect(html).toContain(LOGIN_AT.browserLogin)
  })

  it('points to footer banner when suppressLoginActions', () => {
    const html = renderToStaticMarkup(
      <ToolCard
        block={tool()}
        catalog={catalog}
        suppressLoginActions
        onBrowserLogin={() => {}}
      />,
    )
    expect(html).toContain(LOGIN_AT.useBanner)
    expect(html).not.toContain('tool-card-browser-login')
  })

  it('hides login UI when result is not login_required', () => {
    const html = renderToStaticMarkup(
      <ToolCard
        block={tool({ result: { code: 'other', message: 'x' } })}
        catalog={catalog}
        onBrowserLogin={() => {}}
      />,
    )
    expect(html).not.toContain(LOGIN_AT.browserLogin)
  })
})

describe('ToolCard onBrowserLogin', () => {
  let host: HTMLDivElement
  let root: Root

  beforeEach(() => {
    ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
      true
    host = document.createElement('div')
    document.body.appendChild(host)
  })

  afterEach(() => {
    act(() => {
      root?.unmount()
    })
    host.remove()
    vi.restoreAllMocks()
  })

  function render(el: ReactNode) {
    act(() => {
      root = createRoot(host)
      root.render(el)
    })
  }

  it('click 浏览器登录 calls onBrowserLogin with connector_id', () => {
    const onBrowserLogin = vi.fn()
    render(<ToolCard block={tool()} catalog={catalog} onBrowserLogin={onBrowserLogin} />)
    const btn = Array.from(host.querySelectorAll('button')).find(
      (b) => b.textContent?.trim() === LOGIN_AT.browserLogin,
    )
    expect(btn).toBeTruthy()
    act(() => {
      btn!.click()
    })
    expect(onBrowserLogin).toHaveBeenCalledWith('crm')
  })
})
