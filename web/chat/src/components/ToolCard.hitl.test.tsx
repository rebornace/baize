// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import { ToolCard } from './ToolCard'

// waiting_human 工具块（多余字段按简报构造，组件只读 name/runId/status/arguments/result）。
const block = {
  kind: 'tool',
  name: 'order_query',
  runId: 'r1',
  callId: 'c1',
  args: {},
  status: 'waiting_human',
  startedAt: '2026-01-01T00:00:00Z',
} as any

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
  vi.restoreAllMocks()
})

function render(el: ReactNode) {
  act(() => {
    root = createRoot(host)
    root.render(el)
  })
}

function buttonByText(text: string): HTMLButtonElement | undefined {
  return Array.from(host.querySelectorAll('button')).find(
    (b) => b.textContent?.trim() === text,
  ) as HTMLButtonElement | undefined
}

function setInputValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!
  act(() => {
    setter.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

describe('ToolCard HITL 两段式拒绝链路', () => {
  it('初始只有同意/拒绝，无确认拒绝按钮与留言框', () => {
    render(<ToolCard block={block} catalog={[]} />)
    expect(buttonByText('同意')).toBeTruthy()
    expect(buttonByText('拒绝')).toBeTruthy()
    expect(buttonByText('确认拒绝')).toBeUndefined()
    expect(host.querySelector('input.hitl-comment')).toBeNull()
  })

  it('第一次点拒绝不调用 resumeRun，只展开留言框与确认拒绝', () => {
    const spy = vi.spyOn(api, 'resumeRun').mockResolvedValue({ run_id: 'r1', status: 'running' })
    render(<ToolCard block={block} catalog={[]} />)

    act(() => {
      buttonByText('拒绝')!.click()
    })

    expect(spy).toHaveBeenCalledTimes(0)
    const input = host.querySelector('input.hitl-comment') as HTMLInputElement | null
    expect(input).toBeTruthy()
    expect(input!.placeholder).toContain('留言')
    expect(buttonByText('确认拒绝')).toBeTruthy()
  })

  it('确认拒绝携带 trim 后的 comment，且只调用一次', async () => {
    const spy = vi.spyOn(api, 'resumeRun').mockResolvedValue({ run_id: 'r1', status: 'running' })
    render(<ToolCard block={block} catalog={[]} />)

    act(() => {
      buttonByText('拒绝')!.click()
    })
    const input = host.querySelector('input.hitl-comment') as HTMLInputElement
    setInputValue(input, '  请补充理由  ')

    await act(async () => {
      buttonByText('确认拒绝')!.click()
    })

    expect(spy).toHaveBeenCalledTimes(1)
    expect(spy).toHaveBeenCalledWith('r1', 'reject', '请补充理由')
  })

  it('留言为空时确认拒绝以空字符串 comment 提交', async () => {
    const spy = vi.spyOn(api, 'resumeRun').mockResolvedValue({ run_id: 'r1', status: 'running' })
    render(<ToolCard block={block} catalog={[]} />)

    act(() => {
      buttonByText('拒绝')!.click()
    })

    await act(async () => {
      buttonByText('确认拒绝')!.click()
    })

    expect(spy).toHaveBeenCalledTimes(1)
    expect(spy).toHaveBeenCalledWith('r1', 'reject', '')
  })

  it('直接点同意以空 comment 调用 approve', async () => {
    const spy = vi.spyOn(api, 'resumeRun').mockResolvedValue({ run_id: 'r1', status: 'running' })
    render(<ToolCard block={block} catalog={[]} />)

    await act(async () => {
      buttonByText('同意')!.click()
    })

    expect(spy).toHaveBeenCalledTimes(1)
    expect(spy).toHaveBeenCalledWith('r1', 'approve', '')
  })

  it('即便先展开留言框，同意仍恒以空 comment 提交', async () => {
    const spy = vi.spyOn(api, 'resumeRun').mockResolvedValue({ run_id: 'r1', status: 'running' })
    render(<ToolCard block={block} catalog={[]} />)

    act(() => {
      buttonByText('拒绝')!.click()
    })
    const input = host.querySelector('input.hitl-comment') as HTMLInputElement
    setInputValue(input, '不应被带上的留言')

    await act(async () => {
      buttonByText('同意')!.click()
    })

    expect(spy).toHaveBeenCalledTimes(1)
    expect(spy).toHaveBeenCalledWith('r1', 'approve', '')
  })

  it('resumeRun reject 时回调 onError 一次且卡片只显通用提示、不露 HTTP 技术串', async () => {
    const technical = new Error('HTTP 502: upstream connect failure')
    vi.spyOn(api, 'resumeRun').mockRejectedValue(technical)
    const onError = vi.fn()
    render(<ToolCard block={block} catalog={[]} onError={onError} />)

    await act(async () => {
      buttonByText('同意')!.click()
    })

    expect(onError).toHaveBeenCalledTimes(1)
    expect(onError).toHaveBeenCalledWith(technical)
    const errorEl = host.querySelector('.tool-card-error')
    expect(errorEl?.textContent).toContain('操作失败，请重试')
    expect(host.textContent).not.toContain('HTTP')
  })
})
