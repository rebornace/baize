// @vitest-environment jsdom
import { type ReactNode } from 'react'
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { GoalStackView } from '../../api/goal'
import { GoalBar, type GoalBarProps } from './GoalBar'

let host: HTMLDivElement

const baseView: GoalStackView = {
  conversation_id: 'c1',
  revision: 1,
  frames: [
    {
      id: 'f1',
      text: '主任务',
      status: 'active',
      source: 'user',
      revision: 1,
    },
  ],
  root: {
    id: 'f1',
    text: '主任务',
    status: 'active',
    source: 'user',
    revision: 1,
  },
  tip: {
    id: 'f1',
    text: '主任务',
    status: 'active',
    source: 'user',
    revision: 1,
  },
}

const noopPick = {
  onPickMain: () => {},
  onPickSide: () => {},
} satisfies Pick<GoalBarProps, 'onPickMain' | 'onPickSide'>

beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
})

afterEach(() => host.remove())

function render(el: ReactNode) {
  act(() => {
    createRoot(host).render(el)
  })
}

function openMenu() {
  act(() => {
    host.querySelector<HTMLButtonElement>('.goal-bar-more')?.click()
  })
}

function menuItem(label: string) {
  return Array.from(host.querySelectorAll<HTMLButtonElement>('.goal-bar-menu-item')).find(
    (b) => b.textContent === label,
  )
}

describe('GoalBar', () => {
  it('renders nothing when stack is empty', () => {
    render(
      <GoalBar
        view={{ conversation_id: 'c1', revision: 0, frames: [] }}
        writeDisabled={false}
        onSave={() => {}}
        onPush={() => {}}
        onPop={() => {}}
        onStop={() => {}}
        onResume={() => {}}
        {...noopPick}
      />,
    )
    expect(host.querySelector('[data-testid="goal-bar"]')).toBeNull()
  })

  it('hides empty-stack chrome even when candidates exist', () => {
    render(
      <GoalBar
        view={{ conversation_id: 'c1', revision: 0, frames: [] }}
        candidates={[{ id: 'm1', text: '修好登录', preview: '修好登录' }]}
        writeDisabled={false}
        onSave={() => {}}
        onPush={() => {}}
        onPop={() => {}}
        onStop={() => {}}
        onResume={() => {}}
        {...noopPick}
      />,
    )
    expect(host.querySelector('[data-testid="goal-bar"]')).toBeNull()
  })

  it('shows a single compact summary line', () => {
    render(
      <GoalBar
        view={baseView}
        writeDisabled={false}
        onSave={() => {}}
        onPush={() => {}}
        onPop={() => {}}
        onStop={() => {}}
        onResume={() => {}}
        {...noopPick}
      />,
    )
    expect(host.textContent).toContain('目标')
    expect(host.textContent).toContain('主任务')
    expect(host.querySelector('[data-testid="goal-bar-summary"]')).not.toBeNull()
    expect(host.textContent).toContain('更多')
    expect(host.querySelector('[data-testid="goal-bar-menu"]')).toBeNull()
  })

  it('opens details panel on summary click and edits via save', () => {
    const onSave = vi.fn()
    render(
      <GoalBar
        view={{
          ...baseView,
          frames: [
            {
              id: 'f1',
              text: '主任务',
              details: '需要兼容旧登录',
              status: 'active',
              source: 'user',
              revision: 1,
            },
          ],
          root: {
            id: 'f1',
            text: '主任务',
            details: '需要兼容旧登录',
            status: 'active',
            source: 'user',
            revision: 1,
          },
          tip: {
            id: 'f1',
            text: '主任务',
            details: '需要兼容旧登录',
            status: 'active',
            source: 'user',
            revision: 1,
          },
        }}
        writeDisabled={false}
        onSave={onSave}
        onPush={() => {}}
        onPop={() => {}}
        onStop={() => {}}
        onResume={() => {}}
        {...noopPick}
      />,
    )
    expect(host.textContent).toContain('有详情')
    act(() => {
      host.querySelector<HTMLButtonElement>('[data-testid="goal-bar-summary"]')?.click()
    })
    expect(host.querySelector('[data-testid="goal-bar-detail"]')).not.toBeNull()
    expect(host.querySelector('[data-testid="goal-bar-details"]')?.textContent).toContain(
      '需要兼容旧登录',
    )
    act(() => {
      Array.from(host.querySelectorAll<HTMLButtonElement>('button'))
        .find((b) => b.textContent === '编辑')
        ?.click()
    })
    const title = host.querySelector<HTMLInputElement>('.goal-bar-edit input')
    const details = host.querySelector<HTMLTextAreaElement>('.goal-bar-edit textarea')
    expect(title?.value).toBe('主任务')
    expect(details?.value).toBe('需要兼容旧登录')
    const setInput = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    const setArea = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')?.set
    act(() => {
      if (title && setInput) {
        setInput.call(title, '修好登录')
        title.dispatchEvent(new Event('input', { bubbles: true }))
      }
      if (details && setArea) {
        setArea.call(details, '先查 OAuth')
        details.dispatchEvent(new Event('input', { bubbles: true }))
      }
    })
    act(() => {
      host.querySelector<HTMLFormElement>('[data-testid="goal-bar-edit"]')?.requestSubmit()
    })
    expect(onSave).toHaveBeenCalledWith({
      text: '修好登录',
      details: '先查 OAuth',
      scope: 'root',
    })
  })

  it('fires action callbacks from the overflow menu', () => {
    const onPush = vi.fn()
    const onPop = vi.fn()
    const onStop = vi.fn()
    render(
      <GoalBar
        view={{
          ...baseView,
          frames: [
            { id: 'f1', text: '主任务', status: 'active', source: 'user', revision: 1 },
            { id: 'f2', text: '临时', status: 'active', source: 'user', revision: 1 },
          ],
          tip: {
            id: 'f2',
            text: '临时',
            status: 'active',
            source: 'user',
            revision: 1,
          },
        }}
        writeDisabled={false}
        onSave={() => {}}
        onPush={onPush}
        onPop={onPop}
        onStop={onStop}
        onResume={() => {}}
        {...noopPick}
      />,
    )
    expect(host.textContent).toContain('主任务')
    expect(host.textContent).toContain('临时')
    openMenu()
    menuItem('先做别的')?.click()
    openMenu()
    menuItem('回到上一层')?.click()
    openMenu()
    menuItem('暂停')?.click()
    expect(onPush).toHaveBeenCalledTimes(1)
    expect(onPop).toHaveBeenCalledTimes(1)
    expect(onStop).toHaveBeenCalledTimes(1)
  })

  it('opens side picker from push when candidates exist', () => {
    const onPickSide = vi.fn()
    const onPush = vi.fn()
    render(
      <GoalBar
        view={baseView}
        candidates={[{ id: 'm1', text: '查日志', preview: '查日志' }]}
        writeDisabled={false}
        onSave={() => {}}
        onPush={onPush}
        onPop={() => {}}
        onStop={() => {}}
        onResume={() => {}}
        onPickMain={() => {}}
        onPickSide={onPickSide}
      />,
    )
    openMenu()
    act(() => {
      menuItem('先做别的')?.click()
    })
    expect(onPush).not.toHaveBeenCalled()
    expect(host.querySelector('[data-testid="goal-bar-picker"]')).not.toBeNull()
    act(() => {
      host.querySelector<HTMLButtonElement>('.goal-bar-picker-item')?.click()
    })
    expect(onPickSide).toHaveBeenCalledWith('查日志')
  })

  it('shows load error with retry without dropping stacked view', () => {
    const onRetry = vi.fn()
    render(
      <GoalBar
        view={baseView}
        loadError
        onRetry={onRetry}
        writeDisabled={false}
        onSave={() => {}}
        onPush={() => {}}
        onPop={() => {}}
        onStop={() => {}}
        onResume={() => {}}
        {...noopPick}
      />,
    )
    expect(host.querySelector('[data-testid="goal-bar-error"]')).not.toBeNull()
    expect(host.textContent).toContain('主任务')
    host.querySelector<HTMLButtonElement>('[data-testid="goal-bar-error"] button')?.click()
    expect(onRetry).toHaveBeenCalledTimes(1)
  })

  it('shows error-only bar when load failed and stack empty', () => {
    render(
      <GoalBar
        view={{ conversation_id: 'c1', revision: 0, frames: [] }}
        loadError
        onRetry={() => {}}
        writeDisabled={false}
        onSave={() => {}}
        onPush={() => {}}
        onPop={() => {}}
        onStop={() => {}}
        onResume={() => {}}
        {...noopPick}
      />,
    )
    expect(host.querySelector('[data-testid="goal-bar"]')).not.toBeNull()
    expect(host.querySelector('.goal-bar-more')).toBeNull()
  })

  it('shows resume and paused focus after nested stop; pop does not drop root', () => {
    const onResume = vi.fn()
    const onPop = vi.fn()
    render(
      <GoalBar
        view={{
          conversation_id: 'c1',
          revision: 3,
          frames: [
            { id: 'f1', text: '主任务', status: 'active', source: 'user', revision: 1 },
            { id: 'f2', text: '临时', status: 'interrupted', source: 'user', revision: 2 },
          ],
          root: {
            id: 'f1',
            text: '主任务',
            status: 'active',
            source: 'user',
            revision: 1,
          },
          tip: {
            id: 'f1',
            text: '主任务',
            status: 'active',
            source: 'user',
            revision: 1,
          },
        }}
        writeDisabled={false}
        onSave={() => {}}
        onPush={() => {}}
        onPop={onPop}
        onStop={() => {}}
        onResume={onResume}
        {...noopPick}
      />,
    )
    expect(host.textContent).toContain('主任务')
    expect(host.textContent).toContain('临时')
    openMenu()
    const resumeBtn = menuItem('继续')
    expect(resumeBtn).toBeTruthy()
    resumeBtn?.click()
    expect(onResume).toHaveBeenCalledTimes(1)
    openMenu()
    const popBtn = menuItem('回到上一层')
    expect(popBtn?.disabled).toBe(false)
    popBtn?.click()
    expect(onPop).toHaveBeenCalledTimes(1)
  })

  it('disables write controls when busy', () => {
    render(
      <GoalBar
        view={baseView}
        writeDisabled
        onSave={() => {}}
        onPush={() => {}}
        onPop={() => {}}
        onStop={() => {}}
        onResume={() => {}}
        {...noopPick}
      />,
    )
    expect(host.querySelector<HTMLButtonElement>('.goal-bar-more')?.disabled).toBe(true)
  })
})
