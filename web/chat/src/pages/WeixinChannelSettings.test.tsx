// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import { GateContext } from '../gateContext'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { WEIXIN } from '../strings'
import { WeixinChannelSettings } from './WeixinChannelSettings'

async function renderPage(
  role: 'admin' | 'operator' = 'admin',
  deliveries: api.ChannelOutboundDelivery[] = [],
) {
  setPack(zhPack)
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
  vi.spyOn(api, 'getChannelOutboundDeliveries').mockResolvedValue(deliveries)
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
    expect(host.textContent).toContain(WEIXIN.getQr)
    expect(host.textContent).not.toContain(WEIXIN.saveSettings)
    expect(host.querySelector('textarea')).toBeNull()
    root.unmount()
    host.remove()
  })

  it('asks ConfirmDialog before logout; window.confirm unused', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm')
    const logout = vi.spyOn(api, 'logoutWeixin').mockResolvedValue({ status: 'ok' })
    const { host, root } = await renderPage('admin')

    const logoutBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(WEIXIN.logout),
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

  it('shows outbound deliveries title and retry for dead row', async () => {
    const retry = vi.spyOn(api, 'retryChannelOutboundDelivery').mockResolvedValue({ status: 'queued' })
    const { host, root } = await renderPage('admin', [
      {
        id: 'ob_dead',
        status: 'dead',
        kind: 'text',
        peer_id: 'p1',
        conversation_id: 'weixin:a:p1',
        run_id: 'run1',
        attempt: 5,
        max_attempts: 5,
        last_error: 'timeout',
      },
    ])

    expect(host.textContent).toContain(WEIXIN.outboundTitle)
    expect(host.textContent).not.toMatch(/5xx|死信/)
    const retryBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(WEIXIN.outboundRetry),
    )
    expect(retryBtn).toBeTruthy()

    await act(async () => {
      retryBtn!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(retry).toHaveBeenCalledWith('weixin', 'ob_dead')

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
      b.textContent?.includes(WEIXIN.processStop),
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
