// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { GateContext } from '../gateContext'
import { LocaleProvider } from '../locale/LocaleContext'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { WEIXIN } from '../strings'
import { WeixinChannelSettings } from './WeixinChannelSettings'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
let host: HTMLDivElement
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  setPack(zhPack)
  vi.stubGlobal('fetch', vi.fn())
})
afterEach(() => { host.remove(); vi.unstubAllGlobals(); setPack(zhPack) })

async function renderAs(role: 'admin' | 'operator') {
  vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) => {
    const u = String(url)
    if (u === '/v0/settings/channels/weixin') {
      return jsonResponse({ agent_id: 'a', assignee: 'alice', allowlist: [], enabled: true, running: true })
    }
    return jsonResponse(null)
  })
  await act(async () => {
    createRoot(host).render(
      <MemoryRouter>
        <LocaleProvider>
          <GateContext.Provider value={{ role, gateEnabled: true, operatorId: 'op' }}>
            <WeixinChannelSettings />
          </GateContext.Provider>
        </LocaleProvider>
      </MemoryRouter>,
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
}

describe('WeixinChannelSettings operator', () => {
  it('keeps login QR + status but hides process/logout/settings-form', async () => {
    await renderAs('operator')
    expect(host.textContent).toContain(WEIXIN.loginSection)
    expect(host.textContent).toContain(WEIXIN.getQr)
    expect(host.textContent).not.toContain(WEIXIN.processStart)
    expect(host.textContent).not.toContain(WEIXIN.processRestart)
    expect(host.textContent).not.toContain(WEIXIN.processStop)
    expect(host.textContent).not.toContain(WEIXIN.logout)
    expect(host.textContent).not.toContain(WEIXIN.saveSettings)
    expect(host.querySelector('textarea')).toBeNull()
  })

  it('admin keeps process controls, logout and settings form', async () => {
    await renderAs('admin')
    expect(host.textContent).toContain(WEIXIN.processStart)
    expect(host.textContent).toContain(WEIXIN.logout)
    expect(host.textContent).toContain(WEIXIN.saveSettings)
  })
})
