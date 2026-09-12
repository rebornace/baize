// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { ChannelFileLink, fileKind } from './ChannelFileLink'
import { GateContext } from '../gateContext'

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => host.remove())

function render(el: ReactNode) {
  act(() => {
    createRoot(host).render(
      <GateContext.Provider value={{ role: 'operator', gateEnabled: false, operatorId: 'op' }}>
        {el}
      </GateContext.Provider>,
    )
  })
}

describe('fileKind', () => {
  it('maps common extensions to friendly labels', () => {
    expect(fileKind('photo.JPG').label).toBe('图片')
    expect(fileKind('sheet.xlsx').label).toBe('表格')
    expect(fileKind('data.csv').label).toBe('表格')
    expect(fileKind('bundle.zip').label).toBe('压缩包')
    expect(fileKind('行程.docx').label).toBe('文档')
    expect(fileKind('plan.pdf').label).toBe('文档')
    expect(fileKind('deck.pptx').label).toBe('演示')
  })

  it('falls back to the uppercase extension for unknown types', () => {
    expect(fileKind('archive.7z').label).toBe('压缩包')
    expect(fileKind('script.ps1').label).toBe('PS1')
    expect(fileKind('noext').label).toBe('文件')
  })
})

describe('ChannelFileLink card', () => {
  it('renders an icon, file name, kind and a download affordance (no paperclip text)', () => {
    render(<ChannelFileLink name="黄山三日行程.docx" url="/v0/channels/media/x/y.docx" />)
    const card = host.querySelector('.channel-file-link')
    expect(card).not.toBeNull()
    expect(host.querySelector('.channel-file-name')?.textContent).toBe('黄山三日行程.docx')
    expect(host.querySelector('.channel-file-kind')?.textContent).toBe('文档')
    expect(host.querySelector('.channel-file-icon')).not.toBeNull()
    expect(host.querySelector('.channel-file-dl')).not.toBeNull()
    // Old presentation (emoji + "点击下载" hint) must be gone.
    expect(card?.textContent).not.toContain('📎')
    expect(card?.textContent).not.toContain('点击下载')
  })
})
