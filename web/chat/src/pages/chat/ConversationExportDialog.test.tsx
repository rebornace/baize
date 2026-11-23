// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ConversationExportDialog } from './ConversationExportDialog'

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

describe('ConversationExportDialog', () => {
  it('exports the selected format with optional thinking', () => {
    const onExport = vi.fn()
    render(
      <ConversationExportDialog
        open
        busy={false}
        onClose={() => {}}
        onExport={onExport}
      />,
    )
    const radios = host.querySelectorAll<HTMLInputElement>('input[name="conversation-export-format"]')
    expect(radios).toHaveLength(3)
    expect(radios[0].checked).toBe(true)

    act(() => {
      radios[1].click()
    })
    const thinking = host.querySelector<HTMLInputElement>('.export-thinking input')
    act(() => {
      thinking?.click()
    })
    const download = [...host.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === '下载',
    )
    act(() => {
      download?.click()
    })
    expect(onExport).toHaveBeenCalledWith('json', true)
  })

  it('disables the actions while exporting', () => {
    render(
      <ConversationExportDialog open busy onClose={() => {}} onExport={() => {}} />,
    )
    const buttons = [...host.querySelectorAll('button')]
    expect(buttons.find((b) => b.textContent?.includes('导出中'))?.disabled).toBe(true)
  })
})
