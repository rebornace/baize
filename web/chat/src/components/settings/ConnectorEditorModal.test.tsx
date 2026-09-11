// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ConnectorEditorModal } from './ConnectorEditorModal'

let host: HTMLDivElement
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host) })
afterEach(() => { host.remove() })

const flush = async (n = 3) => { await act(async () => { for (let i = 0; i < n; i++) await new Promise((r) => setTimeout(r, 0)) }) }
const setValue = (el: Element, value: string) => act(async () => {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
  setter.call(el, value); el.dispatchEvent(new Event('input', { bubbles: true })); await new Promise((r) => setTimeout(r, 0))
})
const btn = (txt: string) => [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(txt))!

const emptyInitial = { id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [] }

async function render(props: Partial<Parameters<typeof ConnectorEditorModal>[0]> = {}) {
  await act(async () => {
    createRoot(host).render(
      <ConnectorEditorModal
        kind="openapi" open editing={false} initial={emptyInitial}
        onClose={vi.fn()}
        formatError={(e: unknown) => (e instanceof Error ? e.message : String(e))}
        onSaveInfo={vi.fn(async () => [{ name: 'login' }, { name: 'list_tickets' }])}
        onSavePermissions={vi.fn(async () => {})}
        {...props}
      />,
    )
    await new Promise((r) => setTimeout(r, 0))
  })
}

describe('ConnectorEditorModal step 1', () => {
  it('validates required fields before saving', async () => {
    const onSaveInfo = vi.fn(async () => [])
    await render({ onSaveInfo })
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    expect(host.textContent).toContain('请填写连接编号')
    expect(host.textContent).toContain('请填写服务地址')
    expect(onSaveInfo).not.toHaveBeenCalled()
  })

  it('openapi create requires a spec', async () => {
    const onSaveInfo = vi.fn(async () => [])
    await render({ onSaveInfo })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'o1')
    await setValue(inputs[1], 'https://api.example.com')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    expect(host.textContent).toContain('请上传接口文档或填写文档链接')
    expect(onSaveInfo).not.toHaveBeenCalled()
  })

  it('plugin create saves with id + base url and moves to step 2', async () => {
    const onSaveInfo = vi.fn(async () => [{ name: 'ping' }])
    await render({ kind: 'plugin', onSaveInfo })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'p1')
    await setValue(inputs[1], 'http://127.0.0.1:19090')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    expect(onSaveInfo).toHaveBeenCalledWith(expect.objectContaining({ id: 'p1' }))
    expect(host.textContent).toContain('工具权限')
    expect(host.textContent).toContain('ping')
  })
})

describe('ConnectorEditorModal step 2', () => {
  const editInitial = {
    id: 'o1', baseUrl: 'https://x',
    tools: [{ name: 'login' }, { name: 'create_ticket' }],
    loginNames: ['login'], approvalNames: ['create_ticket'],
  }

  it('opens directly at step 1 but can reach step 2 with saved tools and echoes checkboxes', async () => {
    await render({ editing: true, initial: editInitial })
    // 编辑既有连接：通过「下一步：设置工具权限」进入第二步（无需再次保存）
    await act(async () => { btn('设置工具权限').click(); await new Promise((r) => setTimeout(r, 0)) })
    const boxes = [...host.querySelectorAll('input[type="checkbox"]')] as HTMLInputElement[]
    const loginBox = boxes.find((b) => b.dataset.tool === 'login' && b.dataset.flag === 'login')!
    const approvalBox = boxes.find((b) => b.dataset.tool === 'create_ticket' && b.dataset.flag === 'approval')!
    expect(loginBox.checked).toBe(true)
    expect(approvalBox.checked).toBe(true)
  })

  it('saves selected permission lists', async () => {
    const onSavePermissions = vi.fn(async () => {})
    await render({ editing: true, initial: editInitial, onSavePermissions })
    await act(async () => { btn('设置工具权限').click(); await new Promise((r) => setTimeout(r, 0)) })
    await act(async () => { btn('完成').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    expect(onSavePermissions).toHaveBeenCalledWith('o1', 'https://x', ['login'], ['create_ticket'])
  })
})
