// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { GateContext } from '../gateContext'
import { WeixinChannelSettings } from './WeixinChannelSettings'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}
let host: HTMLDivElement
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  vi.stubGlobal('fetch', vi.fn())
})
afterEach(() => { host.remove(); vi.unstubAllGlobals() })

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
        <GateContext.Provider value={{ role, gateEnabled: true, operatorId: 'op' }}>
          <WeixinChannelSettings />
        </GateContext.Provider>
      </MemoryRouter>,
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
}

describe('WeixinChannelSettings operator', () => {
  it('keeps login QR + status but hides process/logout/settings-form', async () => {
    await renderAs('operator')
    expect(host.textContent).toContain('登录')
    // 扫码登录入口保留
    expect(host.textContent).toContain('获取登录二维码')
    // 进程控制隐藏
    expect(host.textContent).not.toContain('启动进程')
    expect(host.textContent).not.toContain('重启进程')
    expect(host.textContent).not.toContain('停止进程')
    // 登出隐藏
    expect(host.textContent).not.toContain('登出')
    // 设置表单（含保存）隐藏
    expect(host.textContent).not.toContain('保存设置')
    expect(host.querySelector('textarea')).toBeNull()
  })

  it('admin keeps process controls, logout and settings form', async () => {
    await renderAs('admin')
    expect(host.textContent).toContain('启动进程')
    expect(host.textContent).toContain('登出')
    expect(host.textContent).toContain('保存设置')
  })
})
