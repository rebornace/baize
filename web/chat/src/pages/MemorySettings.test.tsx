// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import { MEMORY } from '../strings'
import { MemorySettings } from './MemorySettings'

async function renderMemory() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  await act(async () => {
    root.render(createElement(MemorySettings))
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

function setSelectValue(el: HTMLSelectElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')!.set!
  setter.call(el, value)
  el.dispatchEvent(new Event('change', { bubbles: true }))
}

function setInputValue(el: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
  setter.call(el, value)
  el.dispatchEvent(new Event('input', { bubbles: true }))
}

describe('MemorySettings', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders list entries from the API', async () => {
    vi.spyOn(api, 'listMemory').mockResolvedValue([
      {
        id: 'm1',
        owner_id: 'alice',
        key: 'tea',
        text: '喜欢绿茶',
        source: 'explicit',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
      {
        id: 'm2',
        owner_id: 'alice',
        text: '住在杭州',
        source: 'auto',
        created_at: '2026-01-02T00:00:00Z',
        updated_at: '2026-01-02T00:00:00Z',
      },
    ])

    const { host, root } = await renderMemory()

    expect(host.textContent).toContain(MEMORY.title)
    expect(host.textContent).toContain('喜欢绿茶')
    expect(host.textContent).toContain('住在杭州')
    expect(host.textContent).toContain(MEMORY.sourceExplicit)
    expect(host.textContent).toContain(MEMORY.sourceAuto)

    root.unmount()
    host.remove()
  })

  it('shows empty-state copy when there are no memories', async () => {
    vi.spyOn(api, 'listMemory').mockResolvedValue([])

    const { host, root } = await renderMemory()

    expect(host.textContent).toContain(MEMORY.emptyTitle)
    expect(host.textContent).toContain(MEMORY.emptyDesc)

    root.unmount()
    host.remove()
  })

  it('deletes via ConfirmDialog and calls deleteMemory', async () => {
    vi.spyOn(api, 'listMemory').mockResolvedValue([
      {
        id: 'm1',
        owner_id: 'alice',
        text: '喜欢绿茶',
        source: 'explicit',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
    ])
    const deleteSpy = vi.spyOn(api, 'deleteMemory').mockResolvedValue()
    const confirmSpy = vi.spyOn(window, 'confirm')

    const { host, root } = await renderMemory()

    const deleteBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(MEMORY.delete),
    )
    expect(deleteBtn).toBeTruthy()
    await act(async () => {
      deleteBtn!.click()
    })

    expect(confirmSpy).not.toHaveBeenCalled()
    expect(deleteSpy).not.toHaveBeenCalled()
    expect(host.textContent).toContain(MEMORY.confirmDeleteTitle)

    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(deleteSpy).toHaveBeenCalledWith('m1')
    expect(host.textContent).toContain(MEMORY.toastDeleted)

    root.unmount()
    host.remove()
  })

  it('filters by source and searches key or text', async () => {
    vi.spyOn(api, 'listMemory').mockResolvedValue([
      {
        id: 'm1',
        owner_id: 'alice',
        key: 'preference.tea',
        text: '喜欢绿茶',
        source: 'explicit',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
      {
        id: 'm2',
        owner_id: 'alice',
        key: 'city',
        text: '住在杭州',
        source: 'auto',
        created_at: '2026-01-02T00:00:00Z',
        updated_at: '2026-01-02T00:00:00Z',
      },
    ])

    const { host, root } = await renderMemory()
    const select = host.querySelector('select[name="memory-source"]') as HTMLSelectElement
    const search = host.querySelector('input[name="memory-search"]') as HTMLInputElement
    expect(select).toBeTruthy()
    expect(search).toBeTruthy()

    await act(async () => {
      setSelectValue(select, 'explicit')
    })
    expect(host.textContent).toContain('喜欢绿茶')
    expect(host.textContent).not.toContain('住在杭州')

    await act(async () => {
      setSelectValue(select, 'all')
      setInputValue(search, '杭州')
    })
    expect(host.textContent).toContain('住在杭州')
    expect(host.textContent).not.toContain('喜欢绿茶')

    root.unmount()
    host.remove()
  })
})
