// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import { MCP_EXPORTS } from '../strings'
import {
  identityToForm,
  mcpExportEndpointUrl,
  McpExportSettings,
  toolExportMode,
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

describe('toolExportMode', () => {
  it('treats empty/omitted export as default', () => {
    expect(toolExportMode({ name: 'a', connector_id: 'c' })).toBe('default')
    expect(toolExportMode({ name: 'a', connector_id: 'c', export: '' })).toBe('default')
    expect(toolExportMode({ name: 'a', connector_id: 'c', export: 'default' })).toBe('default')
  })

  it('keeps force modes', () => {
    expect(toolExportMode({ name: 'a', connector_id: 'c', export: 'force_allow' })).toBe(
      'force_allow',
    )
    expect(toolExportMode({ name: 'a', connector_id: 'c', export: 'force_deny' })).toBe(
      'force_deny',
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
afterEach(() => { host.remove(); vi.unstubAllGlobals(); vi.restoreAllMocks() })
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

function mockExportFetches(extra?: (url: string, init?: RequestInit) => Response | undefined) {
  fetchMock.mockImplementation(async (url: unknown, init?: RequestInit) => {
    const u = String(url)
    const override = extra?.(u, init)
    if (override) return override
    if (u.endsWith('/mcp-export')) return json(settings)
    if (u.includes('/identities')) return json(identities)
    if (u.includes('/keys/') && init?.method === 'DELETE') return json({ status: 'ok' })
    if (u.endsWith('/keys')) return json(oneKey)
    if (u.endsWith('/v0/tools') || u.includes('/v0/tools?')) return json({ tools: [] })
    return json({})
  })
}

describe('McpExportSettings page', () => {
  it('asks via ConfirmDialog before revoking a key and toasts on success', async () => {
    mockExportFetches()
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
    mockExportFetches((u, init) => {
      if (u.endsWith('/keys') && init?.method === 'POST') {
        return json({ id: 'k2', name: 'cursor-dev', identity_id: 'ops', token: 'SECRET-TOKEN', prefix: 'mcp_cd' })
      }
      return undefined
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

  it('lists tools for per-tool export and patches force_allow', async () => {
    mockExportFetches()
    vi.spyOn(api, 'listTools').mockResolvedValue([
      {
        name: 'tickets.list',
        title: '查工单',
        description: '列出工单',
        connector_id: 'hr',
        export: 'default',
      },
    ])
    const patch = vi.spyOn(api, 'patchTool').mockResolvedValue({
      name: 'tickets.list',
      title: '查工单',
      connector_id: 'hr',
      export: 'force_allow',
    })

    await renderExport()
    expect(host.textContent).toContain(MCP_EXPORTS.toolsExportTitle)
    expect(host.textContent).toContain('查工单')

    const select = [...host.querySelectorAll('select')].find((el) =>
      el.getAttribute('aria-label')?.includes('查工单'),
    )
    expect(select).toBeTruthy()
    await act(async () => {
      select!.value = 'force_allow'
      select!.dispatchEvent(new Event('change', { bubbles: true }))
      await Promise.resolve()
    })
    await flush()

    expect(patch).toHaveBeenCalledWith('tickets.list', { export: 'force_allow' })
    expect(host.textContent).toContain(MCP_EXPORTS.toastExportSaved)
  })
})
