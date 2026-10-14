// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import { GateContext } from '../gateContext'
import { LocaleProvider } from '../locale/LocaleContext'
import { EXTERNAL_LOGIN } from '../strings'
import { ExternalLoginSettings } from './ExternalLoginSettings'

afterEach(() => {
  vi.restoreAllMocks()
})

async function renderPage(initialEntry = '/settings/external-login') {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  await act(async () => {
    root.render(
      createElement(
        MemoryRouter,
        { initialEntries: [initialEntry] },
        createElement(
          LocaleProvider,
          null,
          createElement(
            GateContext.Provider,
            { value: { role: 'admin', gateEnabled: true, operatorId: 'admin' } },
            createElement(
              Routes,
              null,
              createElement(Route, { path: '/settings/external-login', element: createElement(ExternalLoginSettings) }),
            ),
          ),
        ),
      ),
    )
  })
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0))
  })
  return host
}

describe('ExternalLoginSettings', () => {
  it('renders wizard shell after catalog load', async () => {
    vi.spyOn(api, 'listConnectors').mockImplementation(async (type?: string) =>
      type === 'openapi' ? [{ id: 'mall', type: 'openapi' }] : [],
    )
    vi.spyOn(api, 'getRuntimeSettings').mockResolvedValue({
      effective: {} as api.RuntimeKnobs,
      overridden: {} as api.RuntimeKnobsOverrides,
      public_base_url: '',
      public_base_url_overridden: false,
    })
    vi.spyOn(api, 'listMCPExportIdentities').mockResolvedValue([])
    vi.spyOn(api, 'listMCPExportKeys').mockResolvedValue([])

    const host = await renderPage()
    expect(host.textContent).toContain(EXTERNAL_LOGIN.title)
    expect(host.querySelector('[data-testid="external-login-settings"]')).toBeTruthy()
    expect(host.textContent).toContain(EXTERNAL_LOGIN.step1)
  })

  it('preselects connector from ?connector= query', async () => {
    vi.spyOn(api, 'listConnectors').mockImplementation(async (type?: string) =>
      type === 'openapi' ? [{ id: 'mall', type: 'openapi' }] : [],
    )
    vi.spyOn(api, 'getRuntimeSettings').mockResolvedValue({
      effective: {} as api.RuntimeKnobs,
      overridden: {} as api.RuntimeKnobsOverrides,
      public_base_url: 'http://127.0.0.1:8080',
      public_base_url_overridden: true,
    })
    vi.spyOn(api, 'listMCPExportIdentities').mockResolvedValue([])
    vi.spyOn(api, 'listMCPExportKeys').mockResolvedValue([])
    const getConnector = vi.spyOn(api, 'getConnector').mockResolvedValue({
      id: 'mall',
      type: 'openapi',
      auth: { hosted_login: { enabled: false, methods: [] } },
    })
    vi.spyOn(api, 'fetchHostedLoginCandidates').mockResolvedValue({
      candidates: [{ id: 'tool-login', mode: 'password_form', label: 'Login' }],
    })

    const host = await renderPage('/settings/external-login?connector=mall')
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50))
    })
    const select = host.querySelector('[data-testid="external-login-connector"]') as HTMLSelectElement
    expect(select?.value).toBe('mall')
    expect(getConnector).toHaveBeenCalled()
  })

  it('reopens with previously saved hosted login selected and methods restored', async () => {
    const saved: api.HostedLoginConfig = {
      enabled: true,
      default_method_id: 'pwd',
      methods: [{ id: 'pwd', label: '密码登录', mode: 'password_form', login_tool: 'login' }],
    }
    vi.spyOn(api, 'listConnectors').mockImplementation(async (type?: string) =>
      type === 'openapi'
        ? [{ id: 'mall', type: 'openapi', auth: { hosted_login: saved } }]
        : [{ id: 'other', type: 'http' }],
    )
    vi.spyOn(api, 'getRuntimeSettings').mockResolvedValue({
      effective: {} as api.RuntimeKnobs,
      overridden: {} as api.RuntimeKnobsOverrides,
      public_base_url: 'http://127.0.0.1:8080',
      public_base_url_overridden: true,
    })
    vi.spyOn(api, 'listMCPExportIdentities').mockResolvedValue([])
    vi.spyOn(api, 'listMCPExportKeys').mockResolvedValue([])
    vi.spyOn(api, 'getConnector').mockResolvedValue({
      id: 'mall',
      type: 'openapi',
      auth: { hosted_login: saved },
    })
    const candidates = vi.spyOn(api, 'fetchHostedLoginCandidates')

    const host = await renderPage('/settings/external-login')
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50))
    })
    const select = host.querySelector('[data-testid="external-login-connector"]') as HTMLSelectElement
    expect(select?.value).toBe('mall')
    expect(select?.textContent).toContain(EXTERNAL_LOGIN.configuredSuffix)
    expect(host.textContent).toContain(EXTERNAL_LOGIN.configuredHint)
    expect(host.querySelector('[data-testid="external-login-open-shortcut"]')).toBeTruthy()
    expect(host.querySelector('[data-testid="external-login-edit-config"]')).toBeTruthy()

    await act(async () => {
      ;(host.querySelector('[data-testid="external-login-edit-config"]') as HTMLButtonElement)?.click()
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(host.textContent).toContain('密码登录')
    expect(candidates).not.toHaveBeenCalled()
  })

  it('issues sign-in link from step-1 shortcut for configured connectors', async () => {
    const saved: api.HostedLoginConfig = {
      enabled: true,
      default_method_id: 'pwd',
      methods: [
        {
          id: 'pwd',
          label: '密码登录',
          mode: 'password_form',
          login_tool: 'login',
          identity_ids: ['mei_1'],
        },
      ],
    }
    vi.spyOn(api, 'listConnectors').mockImplementation(async (type?: string) =>
      type === 'openapi' ? [{ id: 'mall', type: 'openapi', auth: { hosted_login: saved } }] : [],
    )
    vi.spyOn(api, 'getRuntimeSettings').mockResolvedValue({
      effective: {} as api.RuntimeKnobs,
      overridden: {} as api.RuntimeKnobsOverrides,
      public_base_url: 'http://127.0.0.1:8080',
      public_base_url_overridden: true,
    })
    vi.spyOn(api, 'listMCPExportIdentities').mockResolvedValue([{ id: 'mei_1', name: 'Ops' }])
    vi.spyOn(api, 'listMCPExportKeys').mockResolvedValue([
      {
        id: 'k1',
        name: 'key',
        identity_id: 'mei_1',
        prefix: 'bz_',
        allowed_tools: [],
        identity_slots: [{ connector_id: 'mall', identity_id: 'mei_1' }],
      },
    ])
    vi.spyOn(api, 'getConnector').mockResolvedValue({
      id: 'mall',
      type: 'openapi',
      auth: { hosted_login: saved },
    })
    const issue = vi.spyOn(api, 'createHostedLoginTestSession').mockResolvedValue({
      session_id: 'sid',
      url: 'http://127.0.0.1:8080/v0/auth/hosted/sid',
    })
    const openSpy = vi.spyOn(window, 'open').mockImplementation(() => null)

    const host = await renderPage('/settings/external-login')
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50))
    })
    await act(async () => {
      ;(host.querySelector('[data-testid="external-login-open-shortcut"]') as HTMLButtonElement)?.click()
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(issue).toHaveBeenCalled()
    expect(openSpy).toHaveBeenCalledWith(
      'http://127.0.0.1:8080/v0/auth/hosted/sid',
      '_blank',
      'noopener,noreferrer',
    )
  })

  it('saves multi-key slots, force_allow login tools, and allowlist merge', async () => {
    const saved: api.HostedLoginConfig = {
      enabled: true,
      default_method_id: 'pwd',
      methods: [
        {
          id: 'pwd',
          label: '密码',
          mode: 'password_form',
          role: 'user',
          login_tool: 'mall.login',
          identity_ids: ['mei_u', 'mei_a'],
        },
        {
          id: 'tok',
          label: '令牌',
          mode: 'token_paste',
          login_tool: 'mall.token',
          identity_ids: ['mei_u'],
        },
      ],
    }
    vi.spyOn(api, 'listConnectors').mockImplementation(async (type?: string) =>
      type === 'openapi' ? [{ id: 'mall', type: 'openapi', auth: { hosted_login: saved } }] : [],
    )
    vi.spyOn(api, 'getRuntimeSettings').mockResolvedValue({
      effective: {} as api.RuntimeKnobs,
      overridden: {} as api.RuntimeKnobsOverrides,
      public_base_url: 'http://127.0.0.1:8080',
      public_base_url_overridden: true,
    })
    vi.spyOn(api, 'listMCPExportIdentities').mockResolvedValue([
      { id: 'mei_u', name: 'User' },
      { id: 'mei_a', name: 'Admin' },
    ])
    vi.spyOn(api, 'listMCPExportKeys').mockResolvedValue([
      {
        id: 'k1',
        name: 'Key1',
        identity_id: 'mei_u',
        allowed_tools: ['mall.get'],
        identity_slots: [{ connector_id: 'mall', identity_id: 'mei_u' }],
        prefix: 'bz_',
        created_at: '',
      },
      {
        id: 'k2',
        name: 'Key2',
        identity_id: 'mei_a',
        allowed_tools: [],
        identity_slots: [],
        prefix: 'bz_',
        created_at: '',
      },
    ])
    vi.spyOn(api, 'getConnector').mockResolvedValue({
      id: 'mall',
      type: 'openapi',
      auth: { hosted_login: saved },
    })
    const putConnector = vi.spyOn(api, 'putConnector').mockResolvedValue({ id: 'mall', type: 'openapi' })
    const patchTool = vi.spyOn(api, 'patchTool').mockResolvedValue({
      name: 'mall.login',
      connector_id: 'mall',
      export: 'force_allow',
    })
    const patchKey = vi.spyOn(api, 'patchMCPExportKey').mockImplementation(async (id, body) => ({
      id,
      name: id,
      identity_id: 'mei_u',
      allowed_tools: body.allowed_tools ?? [],
      identity_slots: body.identity_slots,
      prefix: 'bz_',
      created_at: '',
    }))

    const host = await renderPage('/settings/external-login')
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50))
    })
    await act(async () => {
      ;(host.querySelector('[data-testid="external-login-edit-config"]') as HTMLButtonElement)?.click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await act(async () => {
      const buttons = Array.from(host.querySelectorAll('button'))
      buttons.find((b) => b.textContent?.includes(EXTERNAL_LOGIN.next))?.click()
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(host.querySelector('[data-testid="external-login-identity-methods"]')).toBeTruthy()
    expect(host.querySelector('[data-testid="external-login-export-keys"]')).toBeNull()
    expect(host.textContent).toContain(EXTERNAL_LOGIN.keysAutoHint(2))

    await act(async () => {
      const form = host.querySelector('form')
      form?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
      await new Promise((r) => setTimeout(r, 50))
    })

    expect(putConnector).toHaveBeenCalled()
    const putBody = putConnector.mock.calls[0]?.[1] as { auth?: { hosted_login?: api.HostedLoginConfig } }
    expect(putBody?.auth?.hosted_login?.methods?.[0]?.identity_ids).toEqual(['mei_u', 'mei_a'])
    expect(patchTool).toHaveBeenCalledWith('mall.login', { export: 'force_allow' })
    expect(patchTool).toHaveBeenCalledWith('mall.token', { export: 'force_allow' })
    expect(patchKey).toHaveBeenCalledTimes(2)
    const firstPatch = patchKey.mock.calls[0]
    expect(firstPatch?.[1]?.allowed_tools).toEqual(
      expect.arrayContaining(['mall.get', 'mall.login', 'mall.token']),
    )
    const slots = firstPatch?.[1]?.identity_slots as api.MCPExportIdentitySlot[]
    expect(slots.some((s) => s.connector_id === 'mall' && s.role === 'user' && s.identity_id === 'mei_u')).toBe(true)
    expect(slots.some((s) => s.connector_id === 'mall' && s.role === 'mei_a' && s.identity_id === 'mei_a')).toBe(true)
    expect(slots.some((s) => s.connector_id === 'mall' && !s.role)).toBe(true)
  })
})
