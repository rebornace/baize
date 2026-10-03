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
  it('shows 去登录 when result code is login_required and not readOnly', () => {
    const html = renderToStaticMarkup(<ToolCard block={tool()} catalog={catalog} />)
    expect(html).toContain(LOGIN_AT.goLogin)
    expect(html).toContain('此工具需要先登录')
  })

  it('hides 去登录 in readOnly history', () => {
    const html = renderToStaticMarkup(
      <ToolCard block={tool()} catalog={catalog} readOnly />,
    )
    expect(html).not.toContain(LOGIN_AT.goLogin)
  })

  it('hides 去登录 when result is not login_required', () => {
    const html = renderToStaticMarkup(
      <ToolCard
        block={tool({ result: { code: 'other', message: 'x' } })}
        catalog={catalog}
      />,
    )
    expect(html).not.toContain(LOGIN_AT.goLogin)
  })
})

describe('ToolCard onGoLoginSkill', () => {
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

  it('click 去登录 calls onGoLoginSkill with login-<connector_id>', () => {
    const onGoLoginSkill = vi.fn()
    render(<ToolCard block={tool()} catalog={catalog} onGoLoginSkill={onGoLoginSkill} />)

    const btn = Array.from(host.querySelectorAll('button')).find(
      (b) => b.textContent?.trim() === LOGIN_AT.goLogin,
    )
    expect(btn).toBeTruthy()
    act(() => {
      btn!.click()
    })
    expect(onGoLoginSkill).toHaveBeenCalledTimes(1)
    expect(onGoLoginSkill).toHaveBeenCalledWith('login-crm')
  })

  it('normalizes connector_id the same way as backend SkillID', () => {
    const onGoLoginSkill = vi.fn()
    render(
      <ToolCard
        block={tool()}
        catalog={[{ name: 'order_query', title: '查询订单', connector_id: 'a/b' }]}
        onGoLoginSkill={onGoLoginSkill}
      />,
    )
    const btn = Array.from(host.querySelectorAll('button')).find(
      (b) => b.textContent?.trim() === LOGIN_AT.goLogin,
    )
    expect(btn).toBeTruthy()
    act(() => {
      btn!.click()
    })
    expect(onGoLoginSkill).toHaveBeenCalledWith('login-ab')
  })

  it('does not call onGoLoginSkill when connector_id is missing', () => {
    const onGoLoginSkill = vi.fn()
    render(
      <ToolCard
        block={tool()}
        catalog={[{ name: 'order_query', title: '查询订单' }]}
        onGoLoginSkill={onGoLoginSkill}
      />,
    )
    const btn = Array.from(host.querySelectorAll('button')).find(
      (b) => b.textContent?.trim() === LOGIN_AT.goLogin,
    )
    expect(btn).toBeTruthy()
    act(() => {
      btn!.click()
    })
    expect(onGoLoginSkill).not.toHaveBeenCalled()
  })
})
