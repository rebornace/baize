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
    expect(host.textContent).toContain('请先勾选确认')
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

  it('requires DSN for postgres', async () => {
    await renderPage({ driver: 'postgres', drivers: ['sqlite', 'postgres'] })
    await changeSelect('postgres')
    await act(async () => {
      ;(host.querySelector('input[type="checkbox"]') as HTMLInputElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await fire(findBtn('保存并重启'))
    // 未填 DSN：不进入确认、不发 PUT，提示需要 DSN
    expect(host.querySelector('[data-testid="confirm-ok"]')).toBeNull()
    expect(fetchMock.mock.calls.filter(([, i]) => (i as RequestInit)?.method === 'PUT')).toHaveLength(0)
  })
})
