// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ConnectorEditorModal } from './ConnectorEditorModal'
import { CONNECTORS } from '../../strings'

let host: HTMLDivElement
let root: ReturnType<typeof createRoot>
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host); root = createRoot(host) })
afterEach(() => { act(() => { root.unmount() }); host.remove() })

const flush = async (n = 3) => { await act(async () => { for (let i = 0; i < n; i++) await new Promise((r) => setTimeout(r, 0)) }) }
const setValue = (el: Element, value: string) => act(async () => {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
  setter.call(el, value); el.dispatchEvent(new Event('input', { bubbles: true })); await new Promise((r) => setTimeout(r, 0))
})
const btn = (txt: string) => [...host.querySelectorAll('button')].find((b) => b.textContent!.includes(txt))!

const emptyInitial = { id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [] }
const editInitial = {
  id: 'o1', baseUrl: 'https://x',
  tools: [{ name: 'login' }, { name: 'create_ticket' }],
  loginNames: ['login'], approvalNames: ['create_ticket'],
}

type Props = Parameters<typeof ConnectorEditorModal>[0]
type SaveInfoInput = Parameters<Props['onSaveInfo']>[0]
const baseProps: Props = {
  kind: 'openapi',
  open: true,
  editing: false,
  initial: emptyInitial,
  onClose: vi.fn(),
  formatError: (e: unknown) => (e instanceof Error ? e.message : String(e)),
  onSaveInfo: vi.fn(async () => [{ name: 'login' }, { name: 'list_tickets' }]),
  onSavePermissions: vi.fn(async () => {}),
}

async function render(props: Partial<Props> = {}) {
  const merged = { ...baseProps, ...props }
  await act(async () => {
    root.render(<ConnectorEditorModal {...merged} />)
    await new Promise((r) => setTimeout(r, 0))
  })
  return merged
}

async function rerender(props: Props) {
  await act(async () => {
    root.render(<ConnectorEditorModal {...props} />)
    await new Promise((r) => setTimeout(r, 0))
  })
}

const fileInput = () =>
  host.querySelector('input[type="file"]') as HTMLInputElement
const urlInput = () =>
  [...host.querySelectorAll('input')].find((el) => el.placeholder?.includes('openapi.json')) as HTMLInputElement
const setFile = async (file: File) => {
  const input = fileInput()
  await act(async () => {
    Object.defineProperty(input, 'files', { value: [file], configurable: true })
    input.dispatchEvent(new Event('change', { bubbles: true }))
    await new Promise((r) => setTimeout(r, 0))
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

  it('notifies onSavedInfo once with the new id after step-1 save succeeds', async () => {
    const onSavedInfo = vi.fn()
    await render({ kind: 'plugin', onSavedInfo })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'p1')
    await setValue(inputs[1], 'http://127.0.0.1:19090')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    expect(onSavedInfo).toHaveBeenCalledTimes(1)
    expect(onSavedInfo).toHaveBeenCalledWith(expect.objectContaining({ id: 'p1' }))
  })

  it('does not notify onSavedInfo when step-1 save fails', async () => {
    const onSaveInfo = vi.fn(async () => { throw new Error('boom') })
    const onSavedInfo = vi.fn()
    await render({ kind: 'plugin', onSaveInfo, onSavedInfo, formatError: () => '保存失败' })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'p1')
    await setValue(inputs[1], 'http://127.0.0.1:19090')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    expect(onSavedInfo).not.toHaveBeenCalled()
  })
})

describe('ConnectorEditorModal step 2', () => {
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
    expect(onSavePermissions).toHaveBeenCalledWith('o1', ['login'], ['create_ticket'])
  })

  it('edit: navigating directly to permissions does not notify onSavedInfo', async () => {
    const onSavedInfo = vi.fn()
    await render({ editing: true, initial: editInitial, onSavedInfo })
    // 纯导航（无 PUT、无第一步保存）：不应走「保存成功」通道。
    await act(async () => { btn('设置工具权限').click(); await new Promise((r) => setTimeout(r, 0)) })
    expect(host.textContent).toContain('工具权限')
    expect(onSavedInfo).not.toHaveBeenCalled()
  })
})

describe('ConnectorEditorModal I-1 selection preservation', () => {
  it('create: checkboxes survive back-then-save round trip', async () => {
    const onSaveInfo = vi.fn(async () => [{ name: 'ping' }])
    await render({ kind: 'plugin', onSaveInfo })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'p1')
    await setValue(inputs[1], 'http://127.0.0.1:19090')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    // 第 2 步勾选 ping 的 login
    await act(async () => {
      ;(host.querySelector('input[data-tool="ping"][data-flag="login"]') as HTMLInputElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    // 回到第 1 步再保存
    await act(async () => { btn('上一步').click(); await new Promise((r) => setTimeout(r, 0)) })
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    const loginBox = host.querySelector('input[data-tool="ping"][data-flag="login"]') as HTMLInputElement
    expect(loginBox.checked).toBe(true)
  })

  it('edit: toggled and echoed checkboxes survive back-then-next round trip', async () => {
    await render({ editing: true, initial: editInitial })
    await act(async () => { btn('设置工具权限').click(); await new Promise((r) => setTimeout(r, 0)) })
    // 勾一个初始未勾的 approval：login 工具的 approval
    await act(async () => {
      ;(host.querySelector('input[data-tool="login"][data-flag="approval"]') as HTMLInputElement).click()
      await new Promise((r) => setTimeout(r, 0))
    })
    await act(async () => { btn('上一步').click(); await new Promise((r) => setTimeout(r, 0)) })
    await act(async () => { btn('设置工具权限').click(); await new Promise((r) => setTimeout(r, 0)) })
    expect((host.querySelector('input[data-tool="login"][data-flag="approval"]') as HTMLInputElement).checked).toBe(true)
    // 初始回显的勾仍在
    expect((host.querySelector('input[data-tool="login"][data-flag="login"]') as HTMLInputElement).checked).toBe(true)
    expect((host.querySelector('input[data-tool="create_ticket"][data-flag="approval"]') as HTMLInputElement).checked).toBe(true)
  })
})

describe('ConnectorEditorModal I-2 spec file removal', () => {
  async function fillOpenapiIdUrl() {
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'o1')
    await setValue(inputs[1], 'https://api.example.com')
  }

  it('can remove chosen file, re-enabling the URL input', async () => {
    const onSaveInfo = vi.fn(async () => [{ name: 'ping' }])
    await render({ onSaveInfo })
    await setFile(new File(['{}'], 'openapi.json', { type: 'application/json' }))
    expect(host.textContent).toContain('移除已选文件')
    expect(urlInput().disabled).toBe(true)
    await act(async () => { btn('移除已选文件').click(); await new Promise((r) => setTimeout(r, 0)) })
    expect(host.textContent).not.toContain('已选择文件：openapi.json')
    expect(host.textContent).not.toContain('移除已选文件')
    expect(urlInput().disabled).toBe(false)
    // 无文件无 URL：仍被 spec 必填拦住
    await fillOpenapiIdUrl()
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    expect(host.textContent).toContain('请上传接口文档或填写文档链接')
    expect(onSaveInfo).not.toHaveBeenCalled()
    // 填入 URL 后可保存
    await setValue(urlInput(), 'https://api.example.com/openapi.json')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    expect(onSaveInfo).toHaveBeenCalledTimes(1)
    expect(host.textContent).toContain('工具权限')
  })

  it('a non-empty URL change clears a previously chosen file (double insurance)', async () => {
    await render()
    await setFile(new File(['{"x":1}'], 'spec2.json', { type: 'application/json' }))
    expect(host.textContent).toContain('已选择文件：spec2.json')
    // 正常 UI 下选中文件后 URL 框 disabled；此处直接派发原生 input 事件，
    // 验证 onChange 的双保险逻辑：非空 URL 会清掉已选文件。
    await setValue(urlInput(), 'https://api.example.com/openapi.json')
    expect(host.textContent).not.toContain('已选择文件：spec2.json')
    expect(host.textContent).not.toContain('移除已选文件')
    expect(urlInput().disabled).toBe(false)
  })
})

describe('ConnectorEditorModal I-3 reset only on open', () => {
  it('does not reset filled fields when parent passes a new initial object while open', async () => {
    const props = await render()
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'o1')
    await setValue(inputs[1], 'https://api.example.com')
    // 父级渲染产生新引用、内容相同的 initial，open 保持 true
    await rerender({
      ...props,
      initial: { id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [] },
    })
    const after = host.querySelectorAll('input[type="text"], input:not([type])')
    expect((after[0] as HTMLInputElement).value).toBe('o1')
    expect((after[1] as HTMLInputElement).value).toBe('https://api.example.com')
  })
})

describe('ConnectorEditorModal I-4 error/save-lock/payload paths', () => {
  async function fillAndSaveOpenapi() {
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'o1')
    await setValue(inputs[1], 'https://api.example.com')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
  }

  it('shows inline error when onSaveInfo rejects and allows retry', async () => {
    const onSaveInfo = vi.fn()
      .mockRejectedValueOnce(new Error('boom'))
      .mockResolvedValueOnce([{ name: 'ping' }])
    await render({ onSaveInfo, formatError: () => '保存失败：网络错误' })
    await setValue(urlInput(), 'https://api.example.com/openapi.json')
    await fillAndSaveOpenapi()
    await flush()
    const alert = host.querySelector('.ui-inline-error')
    expect(alert?.textContent).toBe('保存失败：网络错误')
    // 弹窗仍开着：保存按钮还在，且未进入第 2 步
    expect(btn('保存连接')).toBeTruthy()
    expect(host.textContent).not.toContain('工具权限')
    // saving 已复位：可再次点击并成功
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    expect(onSaveInfo).toHaveBeenCalledTimes(2)
    expect(host.textContent).toContain('工具权限')
  })

  it('shows inline error when onSavePermissions rejects and keeps modal open', async () => {
    const onSavePermissions = vi.fn(async () => { throw new Error('boom2') })
    await render({
      editing: true, initial: editInitial, onSavePermissions,
      formatError: () => '权限保存失败',
    })
    await act(async () => { btn('设置工具权限').click(); await new Promise((r) => setTimeout(r, 0)) })
    await act(async () => { btn('完成').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    expect(host.querySelector('.ui-inline-error')?.textContent).toBe('权限保存失败')
    expect(btn('完成')).toBeTruthy()
    expect(onSavePermissions).toHaveBeenCalledTimes(1)
  })

  it('cannot be closed via overlay click or Escape while saving', async () => {
    let release: () => void = () => {}
    const pending = new Promise<void>((resolve) => { release = resolve })
    const onSaveInfo = vi.fn(() => pending.then(() => [{ name: 'ping' }]))
    const onClose = vi.fn()
    await render({ kind: 'plugin', onSaveInfo, onClose })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'p1')
    await setValue(inputs[1], 'http://127.0.0.1:19090')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    const overlay = host.querySelector('[data-testid="modal-overlay"]')!
    await act(async () => {
      overlay.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(onClose).not.toHaveBeenCalled()
    expect(host.textContent).not.toContain('工具权限')
    await act(async () => { release(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    expect(host.textContent).toContain('工具权限')
  })

  it('openapi URL-only save sends spec.url without content', async () => {
    const onSaveInfo = vi.fn(async (_input: SaveInfoInput) => [{ name: 'ping' }])
    await render({ onSaveInfo })
    await setValue(urlInput(), 'https://api.example.com/openapi.json')
    await fillAndSaveOpenapi()
    await flush()
    expect(onSaveInfo).toHaveBeenCalledTimes(1)
    expect(onSaveInfo.mock.calls[0][0]).toEqual(expect.objectContaining({
      kind: 'openapi',
      spec: { content: undefined, url: 'https://api.example.com/openapi.json' },
    }))
  })

  it('openapi file save sends spec.content even if URL field shows a value placeholder flow', async () => {
    const onSaveInfo = vi.fn(async (_input: SaveInfoInput) => [{ name: 'ping' }])
    await render({ onSaveInfo })
    await setFile(new File([JSON.stringify({ openapi: '3.0.0' })], 'openapi.json', { type: 'application/json' }))
    // 文件选中时 URL 输入被清空且 disabled，提交应只含 content
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'o1')
    await setValue(inputs[1], 'https://api.example.com')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    expect(onSaveInfo).toHaveBeenCalledTimes(1)
    expect(onSaveInfo.mock.calls[0][0]).toEqual(expect.objectContaining({
      kind: 'openapi',
      spec: { content: JSON.stringify({ openapi: '3.0.0' }), url: undefined },
    }))
  })

  it('plugin save payload carries no spec key', async () => {
    const onSaveInfo = vi.fn(async (_input: SaveInfoInput) => [{ name: 'ping' }])
    await render({ kind: 'plugin', onSaveInfo })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'p1')
    await setValue(inputs[1], 'http://127.0.0.1:19090')
    await act(async () => { btn('保存连接').click(); await new Promise((r) => setTimeout(r, 0)) })
    await flush()
    const input = onSaveInfo.mock.calls[0][0]
    expect(input).toEqual({ kind: 'plugin', id: 'p1', baseUrl: 'http://127.0.0.1:19090' })
    expect('spec' in input).toBe(false)
  })
})

describe('ConnectorEditorModal minor: file read failure', () => {
  it('shows read-failed error when FileReader errors', async () => {
    await render()
    const spy = vi.spyOn(window.FileReader.prototype, 'readAsText').mockImplementation(function (this: FileReader) {
      // 异步派发 error 事件，触发 onerror，模拟读取失败
      setTimeout(() => { this.dispatchEvent(new Event('error')) }, 0)
    })
    try {
      await setFile(new File(['{}'], 'bad.json', { type: 'application/json' }))
      expect(host.textContent).toContain('读取文件失败')
    } finally {
      spy.mockRestore()
    }
  })

  it('clears a prior read-failed error once a new file is read successfully (M13)', async () => {
    await render()
    const failSpy = vi.spyOn(window.FileReader.prototype, 'readAsText').mockImplementation(function (this: FileReader) {
      setTimeout(() => { this.dispatchEvent(new Event('error')) }, 0)
    })
    await setFile(new File(['{}'], 'bad.json', { type: 'application/json' }))
    expect(host.textContent).toContain('读取文件失败')
    failSpy.mockRestore()

    // 重新选择一个可正常读取的文件：顶部错误条必须消失。
    await setFile(new File(['openapi: 3.0.0'], 'ok.json', { type: 'application/json' }))
    expect(host.textContent).not.toContain('读取文件失败')
    expect(host.textContent).toContain('已选择文件：ok.json')
  })
})

describe('ConnectorEditorModal mcp', () => {
  const mcpProps = (over: Partial<Props> = {}): Props => ({
    ...baseProps, kind: 'mcp', onSaveInfo: vi.fn(async () => [{ name: 'query' }]), ...over,
  })
  const mcpEditInitial = {
    id: 'a1', baseUrl: '',
    mcp: { transport: 'stdio' as const, command: 'npx', args: ['x'] },
    tools: [{ name: 'query' }, { name: 'write' }], loginNames: [], approvalNames: ['write'],
  }

  it('stdio: requires command and submits mcp config, then step2 shows approval only', async () => {
    const onSaveInfo = vi.fn(async () => [{ name: 'query' }])
    await render(mcpProps({ onSaveInfo }))
    // 默认 stdio：不填命令先保存
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'a1')
    await act(async () => { btn('保存连接').click(); await Promise.resolve() })
    expect(host.textContent).toContain('请填写启动命令')
    expect(onSaveInfo).not.toHaveBeenCalled()
    // 填命令
    const cmd = [...host.querySelectorAll('input')].find((i) => i.placeholder === 'npx')!
    await setValue(cmd, 'npx')
    await act(async () => { btn('保存连接').click(); await Promise.resolve() })
    await flush()
    expect(onSaveInfo).toHaveBeenCalledWith({ kind: 'mcp', id: 'a1', mcp: { transport: 'stdio', command: 'npx', args: [] } })
    expect(host.textContent).toContain('工具权限')
    // 第二步没有「需本人登录」复选框，只有审批
    expect(host.querySelector('input[data-flag="login"]')).toBeNull()
    expect(host.querySelector('input[data-tool="query"][data-flag="approval"]')).toBeTruthy()
  })

  it('http: requires url and submits url+headers', async () => {
    const onSaveInfo = vi.fn(async () => [{ name: 'q' }])
    await render(mcpProps({ onSaveInfo }))
    await act(async () => {
      const sel = host.querySelector('select')!
      const setter = Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, 'value')!.set!
      setter.call(sel, 'http'); sel.dispatchEvent(new Event('change', { bubbles: true }))
      await Promise.resolve()
    })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'r1')
    const url = [...host.querySelectorAll('input')].find((i) => (i.placeholder ?? '').includes('mcp'))!
    await setValue(url, 'https://mcp.example.com')
    const headers = host.querySelector('textarea')!
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value')!.set!
      setter.call(headers, 'Authorization=Bearer t'); headers.dispatchEvent(new Event('input', { bubbles: true })); await Promise.resolve()
    })
    await act(async () => { btn('保存连接').click(); await Promise.resolve() })
    await flush()
    expect(onSaveInfo).toHaveBeenCalledWith({
      kind: 'mcp', id: 'r1',
      mcp: { transport: 'http', url: 'https://mcp.example.com', headers: { Authorization: 'Bearer t' } },
    })
  })

  it('finish permissions calls onSavePermissions with approval list only (no login flag)', async () => {
    const onSavePermissions = vi.fn(async () => {})
    await render(mcpProps({ editing: true, initial: mcpEditInitial, onSavePermissions }))
    await act(async () => { btn('设置工具权限').click(); await Promise.resolve() })
    const writeBox = host.querySelector('input[data-tool="write"][data-flag="approval"]') as HTMLInputElement
    expect(writeBox.checked).toBe(true)
    await act(async () => { btn('完成').click(); await Promise.resolve() })
    await flush()
    expect(onSavePermissions).toHaveBeenCalledWith('a1', [], ['write'])
  })

  it('http: empty url shows inline error and does not save', async () => {
    const onSaveInfo = vi.fn(async () => [{ name: 'q' }])
    await render(mcpProps({ onSaveInfo }))
    await act(async () => {
      const sel = host.querySelector('select')!
      const setter = Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, 'value')!.set!
      setter.call(sel, 'http'); sel.dispatchEvent(new Event('change', { bubbles: true }))
      await Promise.resolve()
    })
    const inputs = host.querySelectorAll('input[type="text"], input:not([type])')
    await setValue(inputs[0], 'r1')
    await act(async () => { btn('保存连接').click(); await Promise.resolve() })
    expect(host.textContent).toContain(CONNECTORS.errMcpUrlRequired)
    expect(onSaveInfo).not.toHaveBeenCalled()
  })

  it('edit: navigating directly to permissions does not notify onSavedInfo', async () => {
    const onSavedInfo = vi.fn()
    await render(mcpProps({ editing: true, initial: mcpEditInitial, onSavedInfo }))
    await act(async () => { btn('设置工具权限').click(); await Promise.resolve() })
    expect(host.textContent).toContain('工具权限')
    expect(onSavedInfo).not.toHaveBeenCalled()
  })
})
