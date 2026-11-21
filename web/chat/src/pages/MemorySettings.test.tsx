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

function memory(
  id: string,
  text: string,
  key?: string,
  source: 'explicit' | 'auto' = 'explicit',
) {
  return {
    id,
    owner_id: 'alice',
    key,
    text,
    source,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

function page(start: number, count: number) {
  return Array.from({ length: count }, (_, i) => memory(`m${start + i}`, `记忆 ${start + i}`))
}

function buttonByText(host: HTMLElement, text: string) {
  return [...host.querySelectorAll('button')].find((b) => b.textContent?.includes(text))
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

  it('groups same-key entries and marks conflicts/near-duplicates', async () => {
    vi.spyOn(api, 'listMemory').mockResolvedValue([
      {
        id: 'm1',
        owner_id: 'alice',
        key: 'survey.reporting',
        text: '用户喜欢按完成人数汇报问卷',
        source: 'explicit',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
      {
        id: 'm2',
        owner_id: 'alice',
        key: 'survey.reporting',
        text: '用户喜欢按完成人数汇报',
        source: 'auto',
        created_at: '2026-01-02T00:00:00Z',
        updated_at: '2026-01-02T00:00:00Z',
      },
    ])
    const { host, root } = await renderMemory()
    expect(host.textContent).toContain('survey.reporting')
    expect(host.textContent).toContain(MEMORY.conflictSameKey)
    expect(host.textContent).toContain(MEMORY.nearDup)
    expect(host.querySelectorAll('[data-testid="memory-group"]')).toHaveLength(1)
    root.unmount()
    host.remove()
  })

  it('bulk deletes selected memories', async () => {
    vi.spyOn(api, 'listMemory').mockResolvedValue([
      {
        id: 'm1',
        owner_id: 'alice',
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
    const deleteSpy = vi.spyOn(api, 'deleteMemory').mockResolvedValue()
    const { host, root } = await renderMemory()

    const checks = [...host.querySelectorAll<HTMLInputElement>('input[aria-label="选择这条记忆"]')]
    expect(checks).toHaveLength(2)
    await act(async () => {
      checks[0].click()
      checks[1].click()
    })
    const bulk = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(MEMORY.bulkDelete),
    )
    expect(bulk).toBeTruthy()
    await act(async () => {
      bulk!.click()
    })
    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(deleteSpy).toHaveBeenCalledTimes(2)
    root.unmount()
    host.remove()
  })

  it('searches and filters through the server', async () => {
    const explicit = memory('m1', '喜欢绿茶', 'preference.tea')
    const auto = memory('m2', '住在杭州', 'city', 'auto')
    const rows = [explicit, auto]
    const listSpy = vi.spyOn(api, 'listMemory').mockImplementation(async (opts) => {
      return rows.filter((entry) => {
        if (opts?.source && entry.source !== opts.source) return false
        if (!opts?.q) return true
        const q = opts.q.toLowerCase()
        return (
          (entry.key ?? '').toLowerCase().includes(q) || entry.text.toLowerCase().includes(q)
        )
      })
    })

    const { host, root } = await renderMemory()
    const select = host.querySelector('select[name="memory-source"]') as HTMLSelectElement
    const search = host.querySelector('input[name="memory-search"]') as HTMLInputElement
    expect(select).toBeTruthy()
    expect(search).toBeTruthy()

    await act(async () => {
      setSelectValue(select, 'explicit')
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(listSpy).toHaveBeenLastCalledWith({ limit: 50, source: 'explicit' })
    expect(host.textContent).toContain('喜欢绿茶')
    expect(host.textContent).not.toContain('住在杭州')

    await act(async () => {
      setSelectValue(select, 'all')
      setInputValue(search, '杭州')
    })
    await act(async () => {
      await new Promise((r) => setTimeout(r, 300))
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(listSpy).toHaveBeenLastCalledWith({ limit: 50, q: '杭州' })
    expect(host.textContent).toContain('住在杭州')
    expect(host.textContent).not.toContain('喜欢绿茶')

    root.unmount()
    host.remove()
  })

  it('keeps the search toolbar and shows no-matches for an empty server result', async () => {
    const listSpy = vi
      .spyOn(api, 'listMemory')
      .mockResolvedValueOnce([memory('m1', '喜欢绿茶')])
      .mockResolvedValueOnce([])
    const { host, root } = await renderMemory()

    const search = host.querySelector('input[name="memory-search"]') as HTMLInputElement
    await act(async () => {
      setInputValue(search, '不存在')
    })
    await act(async () => {
      await new Promise((r) => setTimeout(r, 300))
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(listSpy).toHaveBeenLastCalledWith({ limit: 50, q: '不存在' })
    expect(host.textContent).toContain(MEMORY.noMatches)
    expect(host.querySelector('input[name="memory-search"]')).toBeTruthy()

    root.unmount()
    host.remove()
  })

  it('carries the active query into load more', async () => {
    const listSpy = vi.spyOn(api, 'listMemory').mockImplementation(async (opts) => {
      if (opts?.offset) return [memory('m50', 'second page')]
      return page(0, 50)
    })
    const { host, root } = await renderMemory()

    await act(async () => {
      setInputValue(
        host.querySelector('input[name="memory-search"]') as HTMLInputElement,
        'green',
      )
    })
    await act(async () => {
      await new Promise((r) => setTimeout(r, 300))
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(listSpy).toHaveBeenLastCalledWith({ limit: 50, q: 'green' })

    await act(async () => {
      buttonByText(host, MEMORY.loadMore)!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(listSpy).toHaveBeenLastCalledWith({ limit: 50, offset: 50, q: 'green' })

    root.unmount()
    host.remove()
  })

  it('ignores a stale search response', async () => {
    let resolveOld!: (value: Awaited<ReturnType<typeof api.listMemory>>) => void
    let resolveNew!: (value: Awaited<ReturnType<typeof api.listMemory>>) => void
    vi.spyOn(api, 'listMemory').mockImplementation((opts) => {
      if (!opts?.q) return Promise.resolve([memory('m0', 'initial')])
      if (opts.q === 'old') {
        return new Promise((resolve) => {
          resolveOld = resolve
        })
      }
      return new Promise((resolve) => {
        resolveNew = resolve
      })
    })
    const { host, root } = await renderMemory()
    const search = host.querySelector('input[name="memory-search"]') as HTMLInputElement

    await act(async () => {
      setInputValue(search, 'old')
    })
    await act(async () => {
      await new Promise((r) => setTimeout(r, 300))
    })
    await act(async () => {
      setInputValue(search, 'new')
    })
    await act(async () => {
      await new Promise((r) => setTimeout(r, 300))
    })

    await act(async () => {
      resolveNew([memory('m2', 'new result')])
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    await act(async () => {
      resolveOld([memory('m1', 'old result')])
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(host.textContent).toContain('new result')
    expect(host.textContent).not.toContain('old result')

    root.unmount()
    host.remove()
  })

  it('hides load more when the first page is short', async () => {
    vi.spyOn(api, 'listMemory').mockResolvedValue(page(0, 3))
    const { host, root } = await renderMemory()

    expect(host.textContent).toContain(MEMORY.loadedAll(3))
    expect(buttonByText(host, MEMORY.loadMore)).toBeFalsy()
    expect(host.textContent).not.toContain(MEMORY.scopeHint)

    root.unmount()
    host.remove()
  })

  it('loads the first 50 and appends the next page with explicit scope copy', async () => {
    const listSpy = vi
      .spyOn(api, 'listMemory')
      .mockResolvedValueOnce(page(0, 50))
      .mockResolvedValueOnce(page(50, 1))
    const { host, root } = await renderMemory()

    expect(listSpy).toHaveBeenNthCalledWith(1, { limit: 50 })
    expect(host.textContent).toContain(MEMORY.loadedCount(50))
    expect(host.textContent).toContain(MEMORY.scopeHint)
    expect(host.textContent).toContain('记忆 0')
    expect(host.textContent).not.toContain('记忆 50')
    expect(host.querySelector('input[name="memory-search"]')?.getAttribute('placeholder')).toBe(
      MEMORY.searchPlaceholder,
    )

    const loadMore = buttonByText(host, MEMORY.loadMore)
    expect(loadMore).toBeTruthy()
    await act(async () => {
      loadMore!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(listSpy).toHaveBeenNthCalledWith(2, { limit: 50, offset: 50 })
    expect(host.textContent).toContain('记忆 50')
    expect(host.textContent).toContain(MEMORY.loadedAll(51))
    expect(buttonByText(host, MEMORY.loadMore)).toBeFalsy()

    root.unmount()
    host.remove()
  })

  it('keeps selection when loading more and resolves near-duplicates across pages', async () => {
    vi.spyOn(api, 'listMemory')
      .mockResolvedValueOnce([
        memory('m0', '用户喜欢按完成人数汇报问卷', 'survey.reporting'),
        ...page(1, 49),
      ])
      .mockResolvedValueOnce([memory('m50', '用户喜欢按完成人数汇报', 'survey.reporting')])
    const { host, root } = await renderMemory()

    const firstCheck = host.querySelector<HTMLInputElement>(
      'input[aria-label="选择这条记忆"]',
    )
    expect(firstCheck).toBeTruthy()
    await act(async () => {
      firstCheck!.click()
    })

    const loadMore = buttonByText(host, MEMORY.loadMore)
    expect(loadMore).toBeTruthy()
    await act(async () => {
      loadMore!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(firstCheck!.checked).toBe(true)
    expect(host.textContent).toContain('用户喜欢按完成人数汇报')
    expect(host.textContent).toContain(MEMORY.conflictSameKey)
    expect(host.textContent).toContain(MEMORY.nearDup)

    root.unmount()
    host.remove()
  })

  it('keeps the loaded depth when refreshing after deletion', async () => {
    const first = page(0, 50)
    const second = page(50, 50)
    const listSpy = vi
      .spyOn(api, 'listMemory')
      .mockResolvedValueOnce(first)
      .mockResolvedValueOnce(second)
      .mockResolvedValueOnce(first.slice(1))
    const deleteSpy = vi.spyOn(api, 'deleteMemory').mockResolvedValue()
    const { host, root } = await renderMemory()

    await act(async () => {
      buttonByText(host, MEMORY.loadMore)!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(listSpy).toHaveBeenNthCalledWith(2, { limit: 50, offset: 50 })

    await act(async () => {
      buttonByText(host, MEMORY.delete)!.click()
    })
    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(deleteSpy).toHaveBeenCalledWith('m0')
    expect(listSpy).toHaveBeenLastCalledWith({ limit: 100 })

    root.unmount()
    host.remove()
  })

  it('keeps the list when loading more fails', async () => {
    vi.spyOn(api, 'listMemory')
      .mockResolvedValueOnce(page(0, 50))
      .mockRejectedValueOnce(new Error('network'))
    const { host, root } = await renderMemory()

    await act(async () => {
      buttonByText(host, MEMORY.loadMore)!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(host.textContent).toContain(MEMORY.loadMoreFailed)
    expect(host.textContent).toContain('记忆 0')
    expect(buttonByText(host, MEMORY.loadMore)).toBeTruthy()

    root.unmount()
    host.remove()
  })
})
