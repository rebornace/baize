// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
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

  it('shows friendly empty state', async () => {
    await render({ kind: 'plugin', rows: [], loading: false, loadError: null, onCreate: vi.fn(), onEdit: vi.fn(), onDelete: vi.fn() })
    expect(host.textContent).toContain('还没有接入插件服务')
    expect(host.textContent).toContain('接入插件服务')
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
})
