// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import { GateContext } from '../gateContext'
import { TOOLS } from '../strings'
import { ToolsSettings } from './ToolsSettings'

async function renderTools(role: 'admin' | 'operator' = 'admin') {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  await act(async () => {
    root.render(
      createElement(
        MemoryRouter,
        null,
        createElement(
          GateContext.Provider,
          { value: { role, gateEnabled: true, operatorId: role } },
          createElement(ToolsSettings),
        ),
      ),
    )
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

function setNativeValue(el: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const proto = Object.getPrototypeOf(el) as HTMLInputElement | HTMLTextAreaElement
  const desc = Object.getOwnPropertyDescriptor(proto, 'value')
  desc?.set?.call(el, value)
  el.dispatchEvent(new Event('input', { bubbles: true }))
}

const extraTool = {
  name: 'extra_ping',
  title: '自定义 Ping',
  connector_id: 'oa1',
  source: 'extra',
  method: 'GET',
  path: '/ping',
  enabled: true,
  require_login: false,
  input_schema: { type: 'object' },
}

const connectorMeta: api.ConnectorInfo = {
  id: 'oa1',
  type: 'openapi',
  base_url: 'https://api.example.com',
  auth: {
    mode: 'static',
    static: { headers: { Authorization: '${TOKEN}' } },
    capture: {
      tool_name_glob: 'old_login',
      token_json_paths: ['accessToken'],
    },
  },
}

describe('ToolsSettings humanized shell', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders 助手功能 not Tools', async () => {
    vi.spyOn(api, 'listTools').mockResolvedValue([
      {
        name: 'list_tickets',
        title: '查工单',
        connector_id: 'oa1',
        source: 'spec',
        enabled: true,
        require_login: false,
      },
    ])
    vi.spyOn(api, 'getConnector').mockResolvedValue({
      id: 'oa1',
      type: 'openapi',
    } as api.ConnectorInfo)

    const { host, root } = await renderTools('admin')

    expect(host.textContent).toContain(TOOLS.title)
    expect(host.querySelector('h1')?.textContent).not.toBe('Tools')
    expect(host.textContent).not.toMatch(/\bTools\b/)

    root.unmount()
    host.remove()
  })

  it('confirms before deleteConnectorTool', async () => {
    vi.spyOn(api, 'listTools').mockResolvedValue([extraTool])
    vi.spyOn(api, 'getConnector').mockResolvedValue({
      id: 'oa1',
      type: 'openapi',
    } as api.ConnectorInfo)
    const deleteSpy = vi.spyOn(api, 'deleteConnectorTool').mockResolvedValue()
    const confirmSpy = vi.spyOn(window, 'confirm')

    const { host, root } = await renderTools('admin')

    const deleteBtn = [...host.querySelectorAll('button')].find((b) => b.textContent?.includes('删除'))
    expect(deleteBtn).toBeTruthy()
    await act(async () => {
      deleteBtn!.click()
    })

    expect(confirmSpy).not.toHaveBeenCalled()
    expect(deleteSpy).not.toHaveBeenCalled()
    expect(host.textContent).toContain(TOOLS.confirmDeleteTitle)
    expect(host.textContent).toContain(TOOLS.confirmDeleteBody)

    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(deleteSpy).toHaveBeenCalledWith('oa1', 'extra_ping')

    root.unmount()
    host.remove()
  })

  it('opens add-tool Modal instead of drawer', async () => {
    vi.spyOn(api, 'listTools').mockResolvedValue([extraTool])
    vi.spyOn(api, 'getConnector').mockResolvedValue({
      id: 'oa1',
      type: 'openapi',
    } as api.ConnectorInfo)

    const { host, root } = await renderTools('admin')

    const addBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(TOOLS.addTool),
    )
    expect(addBtn).toBeTruthy()
    await act(async () => {
      addBtn!.click()
    })

    expect(host.querySelector('.settings-drawer')).toBeNull()
    expect(host.querySelector('[role="dialog"]')).toBeTruthy()
    expect(host.textContent).toContain(TOOLS.fieldConnector)
    expect(host.textContent).toContain(TOOLS.fieldMethod)
    expect(host.textContent).toContain(TOOLS.fieldPath)
    expect(host.textContent).toContain(TOOLS.fieldSchema)

    root.unmount()
    host.remove()
  })

  it('folds schema behind TOOLS.viewSchema details', async () => {
    vi.spyOn(api, 'listTools').mockResolvedValue([extraTool])
    vi.spyOn(api, 'getConnector').mockResolvedValue({
      id: 'oa1',
      type: 'openapi',
    } as api.ConnectorInfo)

    const { host, root } = await renderTools('admin')

    const editBtn = [...host.querySelectorAll('button')].find((b) => b.textContent?.includes('编辑文案'))
    expect(editBtn).toBeTruthy()
    await act(async () => {
      editBtn!.click()
    })

    const details = [...host.querySelectorAll('details')].find(
      (d) => d.querySelector('summary')?.textContent === TOOLS.viewSchema,
    )
    expect(details).toBeTruthy()
    expect(details!.querySelector('summary')?.textContent).toBe(TOOLS.viewSchema)
    expect(details!.querySelector('pre')?.textContent).toContain('"type"')
    expect(details!.open).toBe(false)

    root.unmount()
    host.remove()
  })
})

describe('CaptureSettingsFields', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('uses humanized labels and starts collapsed in parent advanced details', async () => {
    vi.spyOn(api, 'listTools').mockResolvedValue([extraTool])
    vi.spyOn(api, 'getConnector').mockResolvedValue(connectorMeta)

    const { host, root } = await renderTools('admin')

    const adv = host.querySelector('details.settings-advanced') as HTMLDetailsElement | null
    expect(adv && !adv.open).toBe(true)
    expect(host.textContent).toContain(TOOLS.captureToolGlob)
    expect(host.textContent).not.toContain('token_json_paths')
    expect(host.textContent).toContain(TOOLS.captureTokenPaths)
    expect(host.textContent).toContain(TOOLS.captureLabelPaths)
    expect(host.textContent).toContain(TOOLS.captureHeaderTemplate)
    expect(host.textContent).toContain(TOOLS.captureDefaultScheme)
    expect(adv?.querySelector('summary')?.textContent).toBe(TOOLS.advanced)

    root.unmount()
    host.remove()
  })
})

describe('saveConnectorSettings capture merge', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('PUT body includes mergeAuthWithCapture result', async () => {
    vi.spyOn(api, 'listTools').mockResolvedValue([extraTool])
    vi.spyOn(api, 'getConnector').mockResolvedValue(connectorMeta)
    const put = vi.spyOn(api, 'putConnector').mockResolvedValue({
      ...connectorMeta,
      auth: {
        ...connectorMeta.auth,
        capture: { tool_name_glob: '*login*', token_json_paths: ['accessToken'] },
      },
    })

    const { host, root } = await renderTools('admin')

    const adv = host.querySelector('details.settings-advanced') as HTMLDetailsElement
    expect(adv).toBeTruthy()
    const globInput = [...adv.querySelectorAll('input')].find((el) =>
      (el as HTMLInputElement).placeholder.includes('*login*'),
    ) as HTMLInputElement
    expect(globInput).toBeTruthy()

    await act(async () => {
      setNativeValue(globInput, '*login*')
      await new Promise((r) => setTimeout(r, 0))
    })

    const saveBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('保存 Connector 设置'),
    )
    expect(saveBtn).toBeTruthy()
    await act(async () => {
      saveBtn!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(put).toHaveBeenCalled()
    expect(put.mock.calls[0][1].auth?.capture?.tool_name_glob).toBe('*login*')
    expect(put.mock.calls[0][1].auth?.capture?.token_json_paths).toEqual(['accessToken'])
    expect(put.mock.calls[0][1].auth?.static?.headers?.Authorization).toBe('${TOKEN}')

    root.unmount()
    host.remove()
  })
})
