// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { StorageSettings } from './StorageSettings'

function json(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
}

let host: HTMLDivElement
let fetchMock: ReturnType<typeof vi.fn>
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  fetchMock = vi.fn()
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  host.remove()
  vi.unstubAllGlobals()
})

async function renderPage(getBody: unknown = { driver: 'sqlite', drivers: ['memory', 'sqlite', 'postgres'] }) {
  fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
    if (!init?.method && String(url).endsWith('/settings/store')) return json(getBody)
    return json({ status: 'ok', message: 'restarting' })
  })
  await act(async () => {
    createRoot(host).render(<StorageSettings />)
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
}

const findBtn = (txt: string) =>
  [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(txt))!
const fire = (el: Element) => act(async () => {
  (el as HTMLElement).click()
  await new Promise((r) => setTimeout(r, 0))
  await new Promise((r) => setTimeout(r, 0))
})
const changeSelect = (value: string) => act(async () => {
  const sel = host.querySelector('select')!
  const setter = Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, 'value')!.set!
  setter.call(sel, value)
  sel.dispatchEvent(new Event('change', { bubbles: true }))
  await new Promise((r) => setTimeout(r, 0))
})

describe('StorageSettings', () => {
  it('shows friendly driver options but keeps english values', async () => {
    await renderPage()
    const options = [...host.querySelectorAll('select option')].map((o) => ({
      value: o.getAttribute('value'),
      text: o.textContent,
    }))
    expect(options).toContainEqual({ value: 'sqlite', text: '本地文件（SQLite）' })
    expect(options).toContainEqual({ value: 'memory', text: '内存（重启即清空，仅试用）' })
    expect(host.textContent).toContain('数据库文件路径')
  })

  it('switches fields by driver', async () => {
    await renderPage()
    await changeSelect('postgres')
    expect(host.textContent).toContain('连接地址（DSN）')
    expect(host.textContent).not.toContain('数据库文件路径')
    await changeSelect('memory')
    expect(host.textContent).not.toContain('连接地址（DSN）')
  })

  it('blocks submit until the acknowledgement is checked', async () => {
    await renderPage()
    await fire(findBtn('保存并重启'))
    const ackNode = host.querySelector('[data-testid="storage-ack-error"]')
    expect(ackNode).not.toBeNull()
    expect(ackNode!.textContent).toContain('请先勾选确认')
    expect(fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')).toHaveLength(0)
  })

  it('postgres: shows the ack inline error near the checkbox, not inside the DSN field', async () => {
    await renderPage({ driver: 'postgres', drivers: ['sqlite', 'postgres'] })
    await fire(findBtn('保存并重启'))
    // 独立 ack 行内错误节点出现
    const ackNode = host.querySelector('[data-testid="storage-ack-error"]')
    expect(ackNode).not.toBeNull()
    expect(ackNode!.textContent).toContain('请先勾选确认')
    // DSN Field 的错误槽（ui-field-error）不承载 ack 文案
    const dsnFieldErrors = [...host.querySelectorAll('.ui-field-error')].map((n) => n.textContent)
    expect(dsnFieldErrors.some((t) => t?.includes('请先勾选确认'))).toBe(false)
    expect(dsnFieldErrors).toContain('使用 PostgreSQL 需要填写连接地址（DSN）')
    // 仍不打开确认弹窗、不发 PUT
    expect(host.querySelector('[data-testid="confirm-ok"]')).toBeNull()
    expect(fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')).toHaveLength(0)
  })

  it('postgres: with ack checked and empty DSN, error is attached to the DSN field', async () => {
    await renderPage({ driver: 'postgres', drivers: ['sqlite', 'postgres'] })
    await act(async () => {
      ;(host.querySelector('input[type="checkbox"]') as HTMLInputElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await fire(findBtn('保存并重启'))
    expect(host.querySelector('[data-testid="storage-ack-error"]')).toBeNull()
    const dsnFieldErrors = [...host.querySelectorAll('.ui-field-error')].map((n) => n.textContent)
    expect(dsnFieldErrors).toContain('使用 PostgreSQL 需要填写连接地址（DSN）')
    expect(host.querySelector('[data-testid="confirm-ok"]')).toBeNull()
    expect(fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')).toHaveLength(0)
  })

  it('asks for confirmation; cancel sends nothing, confirm PUTs with ack+restart', async () => {
    await renderPage()
    await act(async () => {
      const cb = host.querySelector('input[type="checkbox"]') as HTMLInputElement
      cb.click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await fire(findBtn('保存并重启'))
    // 确认弹窗出现
    expect(host.querySelector('[data-testid="confirm-ok"]')).not.toBeNull()
    await fire(host.querySelector('[data-testid="confirm-cancel"]')!)
    expect(fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')).toHaveLength(0)

    await fire(findBtn('保存并重启'))
    await fire(host.querySelector('[data-testid="confirm-ok"]')!)
    const puts = fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')
    expect(puts).toHaveLength(1)
    const body = JSON.parse((puts[0][1] as RequestInit).body as string)
    expect(body).toMatchObject({ driver: 'sqlite', acknowledge_no_migrate: true, restart: true })
  })

  it('requires DSN for postgres even when a DSN was previously saved', async () => {
    await renderPage({
      driver: 'postgres',
      drivers: ['sqlite', 'postgres'],
      dsn_redacted: 'postgres://u:***@db:5432/baize',
    })
    // 页面诚实提示已有连接串，但不存在「留空保留原 DSN」
    expect(host.textContent).toContain('已保存：postgres://u:***@db:5432/baize')
    await act(async () => {
      ;(host.querySelector('input[type="checkbox"]') as HTMLInputElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await fire(findBtn('保存并重启'))
    // DSN 留空：不进入确认、不发 PUT，行内提示必填（与 dsn_redacted 是否存在无关）
    expect(host.querySelector('[data-testid="confirm-ok"]')).toBeNull()
    expect(fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')).toHaveLength(0)
    expect(host.textContent).toContain('使用 PostgreSQL 需要填写连接地址（DSN）')
  })

  it('PUTs the newly entered DSN for an already configured postgres', async () => {
    await renderPage({
      driver: 'postgres',
      drivers: ['sqlite', 'postgres'],
      dsn_redacted: 'postgres://u:***@db:5432/baize',
    })
    const nextDSN = 'host=db user=u password=p dbname=baize sslmode=disable'
    await act(async () => {
      const input = host.querySelector('input[type="password"]') as HTMLInputElement
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
      setter.call(input, nextDSN)
      input.dispatchEvent(new Event('input', { bubbles: true }))
      ;(host.querySelector('input[type="checkbox"]') as HTMLInputElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await fire(findBtn('保存并重启'))
    expect(host.querySelector('[data-testid="confirm-ok"]')).not.toBeNull()
    await fire(host.querySelector('[data-testid="confirm-ok"]')!)
    const puts = fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')
    expect(puts).toHaveLength(1)
    const body = JSON.parse((puts[0][1] as RequestInit).body as string)
    expect(body).toMatchObject({ driver: 'postgres', acknowledge_no_migrate: true, restart: true })
    expect(body.dsn).toBe(nextDSN)
  })
})
