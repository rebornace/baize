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

  it('keeps method/path/schema only in tech details, not in edit panel', async () => {
    vi.spyOn(api, 'listTools').mockResolvedValue([extraTool])

    const { host, root } = await renderTools('admin')

    const tech = [...host.querySelectorAll('details')].find(
      (d) => d.querySelector('summary')?.textContent === TOOLS.techDetails,
    )
    expect(tech).toBeTruthy()
    expect(tech!.textContent).toContain('GET /ping')
    expect(tech!.querySelector('pre')?.textContent).toContain('"type"')

    const editBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(TOOLS.editCopy),
    )
    expect(editBtn).toBeTruthy()
    await act(async () => {
      editBtn!.click()
    })

    const edit = host.querySelector('.settings-tool-edit')
    expect(edit).toBeTruthy()
    expect(edit!.textContent).not.toContain('方法 / 路径')
    expect(edit!.querySelector('details')).toBeNull()
    expect(edit!.querySelector('pre.settings-tool-schema')).toBeNull()

    root.unmount()
    host.remove()
  })

  it('admin can toggle enable, require-login and require-approval via checkboxes', async () => {
    const gated = {
      ...extraTool,
      require_login: true,
      require_approval: true,
      description: '探测连通性',
    }
    vi.spyOn(api, 'listTools').mockResolvedValue([gated])
    const patchSpy = vi.spyOn(api, 'patchTool').mockImplementation(async (_name, body) => ({
      ...gated,
      ...body,
    }))

    const { host, root } = await renderTools('admin')

    expect(host.textContent).toContain(TOOLS.description)
    expect(host.textContent).toContain(TOOLS.requireLogin)
    expect(host.textContent).toContain(TOOLS.requireApprovalBadge)
    expect(host.textContent).not.toContain('MCP 导出')
    expect(host.textContent).not.toContain('MCP 写类工具')
    expect(host.querySelectorAll('select').length).toBe(0)
    const boxes = [...host.querySelectorAll('input[type="checkbox"]')] as HTMLInputElement[]
    expect(boxes.length).toBe(3)
    expect(boxes[0].checked).toBe(true)
    expect(boxes[1].checked).toBe(true)
    expect(boxes[2].checked).toBe(true)
    expect(host.querySelector('.settings-tool-sub')).toBeNull()

    await act(async () => {
      boxes[1].click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(patchSpy).toHaveBeenCalledWith('extra_ping', { require_login: false })

    await act(async () => {
      boxes[2].click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(patchSpy).toHaveBeenCalledWith('extra_ping', { require_approval: false })

    const tech = [...host.querySelectorAll('details')].find(
      (d) => d.querySelector('summary')?.textContent === TOOLS.techDetails,
    )
    expect(tech).toBeTruthy()
    expect(tech!.open).toBe(false)
    expect(tech!.textContent).toContain('GET /ping')

    const addBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(TOOLS.addTool),
    )
    expect(addBtn?.className).toContain('secondary')

    root.unmount()
    host.remove()
  })
})

describe('ToolsSettings connector group header', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('no longer shows execution callback / capture / save connector controls', async () => {
    vi.spyOn(api, 'listTools').mockResolvedValue([extraTool])
    const put = vi.spyOn(api, 'putConnector')

    const { host, root } = await renderTools('admin')

    expect(host.querySelector('details.settings-advanced')).toBeNull()
    expect(host.textContent).not.toContain('保存 Connector 设置')
    expect(host.textContent).not.toContain('执行回调 URL')
    expect(host.textContent).not.toContain('统一执行地址')
    expect(host.textContent).not.toContain('企业统一执行地址')
    expect(put).not.toHaveBeenCalled()

    root.unmount()
    host.remove()
  })
})
