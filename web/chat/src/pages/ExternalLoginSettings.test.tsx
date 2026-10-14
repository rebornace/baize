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
})
