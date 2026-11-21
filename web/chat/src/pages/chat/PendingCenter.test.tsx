// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { PendingApproval } from '../../api'
import { PendingCenter } from './PendingCenter'

let host: HTMLDivElement
let root: Root

beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
})

afterEach(() => {
  act(() => {
    root?.unmount()
  })
  host.remove()
})

function render(el: ReactNode) {
  act(() => {
    root = createRoot(host)
    root.render(el)
  })
}

function button(text: string): HTMLButtonElement | undefined {
  return Array.from(host.querySelectorAll('button')).find(
    (b) => b.textContent?.trim() === text,
  ) as HTMLButtonElement | undefined
}

async function typeComment(value: string) {
  const input = host.querySelector('.pending-comment') as HTMLInputElement
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
  act(() => {
    setter?.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

const items: PendingApproval[] = [
  {
    run_id: 'r1',
    conversation_id: 'c1',
    conversation_title: '排查 VPN 连接',
    kind: 'approval',
    tool_name: 'send_email',
    prompt: '发送邮件给客户？',
    arguments: { to: 'customer@example.com' },
    waiting_since: new Date(Date.now() - 2 * 60_000).toISOString(),
    workflow_skill: 'ticket-triage',
    workflow_step: 'notify',
  },
  {
    run_id: 'r2',
    conversation_id: 'c2',
    conversation_title: '导出报表',
    kind: 'login',
    prompt: '请完成登录',
    origin: 'scheduled',
  },
]

const noopDecide = async () => {}

describe('PendingCenter', () => {
  it('renders waiting runs with title, kind, tool, prompt and waited time', () => {
    render(
      <PendingCenter
        open
        items={items}
        onClose={() => {}}
        onOpenConversation={() => {}}
        onDecide={noopDecide}
      />,
    )
    expect(host.textContent).toContain('排查 VPN 连接')
    expect(host.textContent).toContain('待审批')
    expect(host.textContent).toContain('send_email')
    expect(host.textContent).toContain('发送邮件给客户？')
    expect(host.textContent).toContain('ticket-triage · notify')
    expect(host.textContent).toContain('已等待')
    expect(host.textContent).toContain('待登录')
    expect(host.textContent).toContain('定时任务')
  })

  it('shows tool arguments and approves with the typed comment', async () => {
    expect(items[0].arguments).toBeDefined()
    const onDecide = vi.fn().mockResolvedValue(undefined)
    render(
      <PendingCenter
        open
        items={items}
        onClose={() => {}}
        onOpenConversation={() => {}}
        onDecide={onDecide}
      />,
    )
    expect(host.textContent).toContain('customer@example.com')

    await typeComment('已核对，同意')
    const approve = button('同意')
    expect(approve).toBeTruthy()
    await act(async () => {
      approve?.click()
    })
    expect(onDecide).toHaveBeenCalledWith('r1', 'approve', '已核对，同意')
  })

  it('rejects only after a second confirmation click', async () => {
    const onDecide = vi.fn().mockResolvedValue(undefined)
    render(
      <PendingCenter
        open
        items={items}
        onClose={() => {}}
        onOpenConversation={() => {}}
        onDecide={onDecide}
      />,
    )
    act(() => {
      button('拒绝')?.click()
    })
    expect(onDecide).not.toHaveBeenCalled()
    const confirm = button('确认拒绝')
    expect(confirm).toBeTruthy()
    await act(async () => {
      confirm?.click()
    })
    expect(onDecide).toHaveBeenCalledWith('r1', 'reject', '')
  })

  it('shows a row error when the decision fails', async () => {
    const onDecide = vi.fn().mockRejectedValue(new Error('boom'))
    render(
      <PendingCenter
        open
        items={items}
        onClose={() => {}}
        onOpenConversation={() => {}}
        onDecide={onDecide}
      />,
    )
    const approve = button('同意')
    await act(async () => {
      approve?.click()
    })
    expect(host.querySelector('[role="alert"]')?.textContent).toContain('操作失败，请重试。')
  })

  it('keeps login waits to "open chat" only', () => {
    render(
      <PendingCenter
        open
        items={[items[1]]}
        onClose={() => {}}
        onOpenConversation={() => {}}
        onDecide={noopDecide}
      />,
    )
    expect(button('同意')).toBeUndefined()
    expect(button('拒绝')).toBeUndefined()
    expect(button('打开会话')).toBeTruthy()
    expect(host.textContent).toContain('登录等待请打开会话处理。')
  })

  it('opens the clicked conversation', () => {
    const onOpen = vi.fn()
    render(
      <PendingCenter
        open
        items={items}
        onClose={() => {}}
        onOpenConversation={onOpen}
        onDecide={noopDecide}
      />,
    )
    const open = button('打开会话')
    expect(open).toBeTruthy()
    act(() => {
      open?.click()
    })
    expect(onOpen).toHaveBeenCalledWith('c1')
  })

  it('shows the empty state', () => {
    render(
      <PendingCenter
        open
        items={[]}
        onClose={() => {}}
        onOpenConversation={() => {}}
        onDecide={noopDecide}
      />,
    )
    expect(host.textContent).toContain('暂时没有待审批或等待中的任务。')
  })
})
