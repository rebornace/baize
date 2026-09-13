// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import { GateContext } from '../gateContext'
import { WEIXIN } from '../strings'
import { WeixinChannelSettings } from './WeixinChannelSettings'

async function renderPage(role: 'admin' | 'operator' = 'admin') {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  vi.spyOn(api, 'getWeixinSettings').mockResolvedValue({
    agent_id: 'a',
    assignee: 'alice',
    allowlist: [],
    enabled: true,
    running: true,
  })
  await act(async () => {
    root.render(
      createElement(
        GateContext.Provider,
        { value: { role, gateEnabled: true, operatorId: 'op' } },
        createElement(WeixinChannelSettings),
      ),
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

describe('WeixinChannelSettings UI', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows humanized PageHeader title 微信 and nav description', async () => {
    const { host, root } = await renderPage('admin')
    expect(host.querySelector('h1')?.textContent).toBe(WEIXIN.title)
    expect(host.textContent).toContain(WEIXIN.description)
    expect(host.querySelector('h1')?.textContent).not.toContain('渠道 ·')
    root.unmount()
    host.remove()
  })

  it('operator keeps login but hides settings form (regression)', async () => {
    const { host, root } = await renderPage('operator')
    expect(host.textContent).toContain('获取登录二维码')
    expect(host.textContent).not.toContain('保存设置')
    expect(host.querySelector('textarea')).toBeNull()
    root.unmount()
    host.remove()
  })

  it('asks ConfirmDialog before logout; window.confirm unused', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm')
    const logout = vi.spyOn(api, 'logoutWeixin').mockResolvedValue({ status: 'ok' })
    const { host, root } = await renderPage('admin')

    const logoutBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('登出'),
    )
    expect(logoutBtn).toBeTruthy()
    await act(async () => {
      logoutBtn!.click()
    })

    expect(confirmSpy).not.toHaveBeenCalled()
    expect(logout).not.toHaveBeenCalled()
    expect(host.textContent).toContain(WEIXIN.confirmLogoutTitle)

    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(logout).toHaveBeenCalled()

    root.unmount()
    host.remove()
  })

  it('asks ConfirmDialog before stop process', async () => {
    const stop = vi.spyOn(api, 'stopWeixinProcess').mockResolvedValue({
      agent_id: 'a',
      assignee: 'alice',
      allowlist: [],
      enabled: true,
      running: false,
      reason: 'stopped',
    })
    const { host, root } = await renderPage('admin')

    const stopBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('停止进程'),
    )
    expect(stopBtn).toBeTruthy()
    await act(async () => {
      stopBtn!.click()
    })
    expect(stop).not.toHaveBeenCalled()
    expect(host.textContent).toContain(WEIXIN.confirmStopTitle)

    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(stop).toHaveBeenCalled()

    root.unmount()
    host.remove()
  })
})
