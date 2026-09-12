// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../../api'
import { ConnectorShell, type ConnectorRowData } from './ConnectorShell'

let host: HTMLDivElement
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host) })
afterEach(() => { host.remove() })

const btn = (txt: string) => [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(txt))!

async function render(props: Parameters<typeof ConnectorShell>[0]) {
  await act(async () => {
    createRoot(host).render(
      <MemoryRouter><ConnectorShell {...props} /></MemoryRouter>,
    )
    await new Promise((r) => setTimeout(r, 0))
  })
}

const rows: ConnectorRowData[] = [
  { id: 'ticket-api', baseUrl: 'https://api.example.com', toolCount: 3, loginNames: ['login'], approvalNames: ['create_ticket'] },
]

describe('ConnectorShell', () => {
  it('renders openapi header, add button and a card with permission summary', async () => {
    await render({ kind: 'openapi', rows, loading: false, loadError: null, onCreate: vi.fn(), onEdit: vi.fn(), onDelete: vi.fn() })
    expect(host.textContent).toContain('业务系统')
    expect(host.textContent).toContain('ticket-api')
    expect(host.textContent).toContain('3 个工具')
    expect(host.textContent).toContain('1 个工具需本人登录 · 1 个需审批')
    expect(btn('接入业务系统')).toBeTruthy()
  })

  it('shows friendly empty state with a single add CTA (no duplicate header button)', async () => {
    await render({ kind: 'plugin', rows: [], loading: false, loadError: null, onCreate: vi.fn(), onEdit: vi.fn(), onDelete: vi.fn() })
    expect(host.textContent).toContain('还没有接入插件服务')
    const addButtons = [...host.querySelectorAll('button')].filter((b) => b.textContent!.includes('接入插件服务'))
    expect(addButtons).toHaveLength(1)
  })

  it('opens row menu and confirms delete', async () => {
    const onDelete = vi.fn().mockResolvedValue(undefined)
    await render({ kind: 'openapi', rows, loading: false, loadError: null, onCreate: vi.fn(), onEdit: vi.fn(), onDelete })
    await act(async () => {
      ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await act(async () => {
      ;[...host.querySelectorAll('.dropdown-item')].find((i) => i.textContent!.includes('删除'))!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(host.textContent).toContain('删除这个连接？')
    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLElement).click()
      await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0))
    })
    expect(onDelete).toHaveBeenCalledWith('ticket-api')
  })

  it('keeps the confirm dialog open and surfaces an error when delete fails, then retries successfully', async () => {
    const unhandled: PromiseRejectionEvent[] = []
    const onUnhandled = (e: PromiseRejectionEvent) => { unhandled.push(e) }
    window.addEventListener('unhandledrejection', onUnhandled)
    const onDelete = vi.fn()
      .mockRejectedValueOnce(new ApiError(500, 'internal_error', 'x'))
      .mockResolvedValueOnce(undefined)
    try {
      await render({ kind: 'openapi', rows, loading: false, loadError: null, onCreate: vi.fn(), onEdit: vi.fn(), onDelete })
      await act(async () => {
        ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
        await new Promise((r) => setTimeout(r, 0))
      })
      await act(async () => {
        ;[...host.querySelectorAll('.dropdown-item')].find((i) => i.textContent!.includes('删除'))!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
        await new Promise((r) => setTimeout(r, 0))
      })
      await act(async () => {
        ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLElement).click()
        await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0))
      })
      // 弹窗仍在、错误文案出现、错误被 catch
      expect(host.querySelector('[data-testid="confirm-ok"]')).toBeTruthy()
      expect(host.textContent).toContain('操作未能完成，请稍后重试。')
      expect(host.textContent).not.toContain('internal_error:')
      expect(unhandled).toHaveLength(0)

      // 再点一次，这次成功，弹窗关闭
      await act(async () => {
        ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLElement).click()
        await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0))
      })
      expect(host.querySelector('[data-testid="confirm-ok"]')).toBeFalsy()
      expect(onDelete).toHaveBeenCalledTimes(2)
      expect(onDelete).toHaveBeenNthCalledWith(2, 'ticket-api')
    } finally {
      window.removeEventListener('unhandledrejection', onUnhandled)
    }
  })

  it('clears a previous delete error when canceling and opening the menu again', async () => {
    const onDelete = vi.fn()
      .mockRejectedValueOnce(new ApiError(500, 'internal_error', 'x'))
      .mockResolvedValueOnce(undefined)
    await render({ kind: 'openapi', rows, loading: false, loadError: null, onCreate: vi.fn(), onEdit: vi.fn(), onDelete })
    await act(async () => {
      ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await act(async () => {
      ;[...host.querySelectorAll('.dropdown-item')].find((i) => i.textContent!.includes('删除'))!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await new Promise((r) => setTimeout(r, 0))
    })
    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLElement).click()
      await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0))
    })
    expect(host.textContent).toContain('操作未能完成，请稍后重试。')
    // 取消后错误消失
    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-cancel"]') as HTMLElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(host.textContent).not.toContain('操作未能完成，请稍后重试。')
    // 再次发起删除，错误已被清空
    await act(async () => {
      ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await act(async () => {
      ;[...host.querySelectorAll('.dropdown-item')].find((i) => i.textContent!.includes('删除'))!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(host.textContent).not.toContain('操作未能完成，请稍后重试。')
    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLElement).click()
      await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0)); await new Promise((r) => setTimeout(r, 0))
    })
    expect(host.querySelector('[data-testid="confirm-ok"]')).toBeFalsy()
  })

  it('calls onEdit with the row id from the edit menu item', async () => {
    const onEdit = vi.fn()
    await render({ kind: 'openapi', rows, loading: false, loadError: null, onCreate: vi.fn(), onEdit, onDelete: vi.fn() })
    await act(async () => {
      ;(host.querySelector('[data-testid="dropdown-trigger"]') as HTMLElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await act(async () => {
      ;[...host.querySelectorAll('.dropdown-item')].find((i) => i.textContent!.includes('编辑'))!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(onEdit).toHaveBeenCalledWith('ticket-api')
  })

  it('renders mcp kind title and a transport summary row without a base url', async () => {
    const mcpRows: ConnectorRowData[] = [
      { id: 'a1', summary: '本地程序 · npx', toolCount: 2, loginNames: [], approvalNames: ['t'] },
    ]
    await render({ kind: 'mcp', rows: mcpRows, loading: false, loadError: null, onCreate: vi.fn(), onEdit: vi.fn(), onDelete: vi.fn() })
    expect(host.textContent).toContain('外部工具服务')
    expect(host.textContent).toContain('本地程序 · npx')
    expect(host.textContent).toContain('1 个需审批')
    expect(host.textContent).not.toContain('需本人登录')
  })

  it('empty mcp list shows the mcp empty state with a single add CTA', async () => {
    await render({ kind: 'mcp', rows: [], loading: false, loadError: null, onCreate: vi.fn(), onEdit: vi.fn(), onDelete: vi.fn() })
    expect(host.textContent).toContain('还没有接入外部工具服务')
    const addButtons = [...host.querySelectorAll('button')].filter((b) => b.textContent!.includes('接入外部工具'))
    expect(addButtons).toHaveLength(1)
  })
})
