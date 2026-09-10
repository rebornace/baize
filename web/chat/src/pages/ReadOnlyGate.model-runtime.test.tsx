// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { GateContext } from '../gateContext'
import { ModelSettings } from './ModelSettings'
import { RuntimeSettings } from './RuntimeSettings'

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

async function renderOperator(el: React.ReactNode) {
  await act(async () => {
    createRoot(host).render(
      <MemoryRouter>
        <GateContext.Provider value={{ role: 'operator', gateEnabled: true, operatorId: 'op' }}>
          {el}
        </GateContext.Provider>
      </MemoryRouter>,
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
}

describe('ModelSettings read-only for operator', () => {
  it('lists profiles but hides create form, edit and delete buttons', async () => {
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) =>
      String(url) === '/v0/settings/models'
        ? jsonResponse({ profiles: [{ id: 'm1', name: '标准模型', model: 'gpt', base_url: 'u', provider: 'openai_compatible', supports_vision: false, disable_thinking: false, context_tokens: 1, auto_tier: 'standard' }] })
        : jsonResponse(null))
    await renderOperator(<ModelSettings />)
    expect(host.textContent).toContain('标准模型')
    expect(host.textContent).not.toContain('添加模型')
    expect(host.querySelector('.settings-form')).toBeNull()
    expect(host.textContent).not.toContain('编辑')
    expect(host.textContent).not.toContain('删除')
  })
})

describe('RuntimeSettings read-only for operator', () => {
  it('shows engine values but hides save button and entire credentials section', async () => {
    const urls: string[] = []
    vi.mocked(globalThis.fetch).mockImplementation(async (url: unknown) => {
      urls.push(String(url))
      const u = String(url)
      if (u === '/v0/settings/runtime') return jsonResponse({ effective: { max_messages: 20 }, overridden: {} })
      if (u === '/v0/settings/credentials') return new Response('forbidden', { status: 403 })
      return jsonResponse(null)
    })
    await renderOperator(<RuntimeSettings />)
    expect(host.textContent).toContain('引擎参数')
    expect(host.textContent).not.toContain('保存引擎参数')
    expect(host.textContent).not.toContain('控制面凭据')
    expect(host.textContent).not.toContain('轮换主口令')
    expect(urls.some((u) => u.includes('/v0/settings/credentials'))).toBe(false)
  })
})
