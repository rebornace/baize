// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MCP_EXPORTS } from '../strings'
import {
  identityToForm,
  mcpExportEndpointUrl,
  McpExportSettings,
  validateIdentityForm,
} from './McpExportSettings'

describe('mcpExportEndpointUrl', () => {
  it('joins origin and path', () => {
    expect(mcpExportEndpointUrl('https://example.com', '/v0/mcp/export')).toBe(
      'https://example.com/v0/mcp/export',
    )
  })

  it('strips trailing slash on origin', () => {
    expect(mcpExportEndpointUrl('https://example.com/', '/v0/mcp/export')).toBe(
      'https://example.com/v0/mcp/export',
    )
  })

  it('ensures leading slash on path', () => {
    expect(mcpExportEndpointUrl('https://example.com', 'v0/mcp/export')).toBe(
      'https://example.com/v0/mcp/export',
    )
  })
})

describe('validateIdentityForm', () => {
  it('requires name', () => {
    expect(validateIdentityForm({ name: '  ', scheme: '', headersText: '' })).toEqual({
      ok: false,
      message: MCP_EXPORTS.errNameRequired,
    })
  })

  it('parses headers via parseKeyValueLines', () => {
    expect(
      validateIdentityForm({
        name: 'Ops',
        scheme: 'Bearer',
        headersText: 'X-Team=ops\nAuthorization=Bearer t',
      }),
    ).toEqual({
      ok: true,
      name: 'Ops',
      scheme: 'Bearer',
      headers: { 'X-Team': 'ops', Authorization: 'Bearer t' },
    })
  })

  it('rejects invalid header lines', () => {
    expect(
      validateIdentityForm({ name: 'Ops', scheme: '', headersText: 'no-equals' }),
    ).toEqual({
      ok: false,
      message: MCP_EXPORTS.errBadHeaderLine('no-equals'),
    })
  })
})

describe('identityToForm', () => {
  it('formats headers as KEY=VALUE lines', () => {
    expect(
      identityToForm({
        id: 'mei_1',
        name: 'Ops',
        scheme: 'Bearer',
        headers: { 'X-Team': 'ops' },
      }),
    ).toEqual({
      name: 'Ops',
      scheme: 'Bearer',
      headersText: 'X-Team=ops',
    })
  })
})

function json(body: unknown, init?: ResponseInit) {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' }, ...init })
}
let host: HTMLDivElement
let fetchMock: ReturnType<typeof vi.fn>
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host); fetchMock = vi.fn(); vi.stubGlobal('fetch', fetchMock) })
afterEach(() => { host.remove(); vi.unstubAllGlobals() })
const flush = async (n = 3) => { await act(async () => { for (let i = 0; i < n; i++) await new Promise((r) => setTimeout(r, 0)) }) }
const btn = (t: string) => [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(t))!
const setValue = (el: Element, v: string) => act(async () => {
  const s = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
  s.call(el, v); el.dispatchEvent(new Event('input', { bubbles: true })); await new Promise((r) => setTimeout(r, 0))
})
async function renderExport() {
  await act(async () => { createRoot(host).render(<MemoryRouter><McpExportSettings /></MemoryRouter>); await Promise.resolve() })
  await flush()
}

const settings = { enabled: true, endpoint_path: '/v0/mcp/export' }
const identities = [{ id: 'ops', name: 'Ops', scheme: 'Bearer', headers: {} }]
const oneKey = [{ id: 'k1', name: 'cursor-dev', identity_id: 'ops', prefix: 'mcp_ab', revoked_at: null }]

describe('McpExportSettings page', () => {
  it('asks via ConfirmDialog before revoking a key and toasts on success', async () => {
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (u.endsWith('/mcp-export')) return json(settings)
      if (u.includes('/identities')) return json(identities)
      if (u.includes('/keys/') && init?.method === 'DELETE') return json({ status: 'ok' })
      if (u.endsWith('/keys')) return json(oneKey)
      return json({})
    })
    await renderExport()
    await act(async () => { btn('撤销').click(); await Promise.resolve() })
    // 确认弹窗出现（复用 Modal）
    expect(host.textContent).toContain('撤销这把密钥？')
    await act(async () => {
      (host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await Promise.resolve()
    })
    await flush()
    const del = fetchMock.mock.calls.find(([u, i]) => String(u).includes('/keys/k1') && (i as RequestInit)?.method === 'DELETE')
    expect(del).toBeTruthy()
    expect(host.textContent).toContain('已撤销密钥 cursor-dev')
  })

  it('shows the one-time token in a Modal with a copy button', async () => {
    fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
      const u = String(url)
      if (u.endsWith('/mcp-export')) return json(settings)
      if (u.endsWith('/identities')) return json(identities)
      if (u.includes('/keys/') && init?.method === 'DELETE') return json({ status: 'ok' })
      if (u.endsWith('/keys')) {
        if (init?.method === 'POST') {
          return json({ id: 'k2', name: 'cursor-dev', identity_id: 'ops', token: 'SECRET-TOKEN', prefix: 'mcp_cd' })
        }
        return json(oneKey)
      }
      return json({})
    })
    await renderExport()
    // 「新建密钥」区名称输入：取最后一个文本输入框
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[inputs.length - 1], 'cursor-dev')
    await act(async () => { btn('新建密钥').click(); await Promise.resolve() })
    await flush()
    expect(host.textContent).toContain('密钥仅显示这一次')
    expect(host.textContent).toContain('SECRET-TOKEN')
    expect(btn('复制密钥')).toBeTruthy()
  })
})
