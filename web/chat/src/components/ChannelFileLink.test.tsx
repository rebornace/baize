// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GateContext } from '../gateContext'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { CHAT } from '../strings'
import { ChannelFileLink, fileKind } from './ChannelFileLink'

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  setPack(zhPack)
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => {
  host.remove()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  setPack(zhPack)
})

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
    expect(fileKind('photo.JPG').label).toBe(CHAT.fileKindImage)
    expect(fileKind('sheet.xlsx').label).toBe(CHAT.fileKindSheet)
    expect(fileKind('data.csv').label).toBe(CHAT.fileKindSheet)
    expect(fileKind('bundle.zip').label).toBe(CHAT.fileKindArchive)
    expect(fileKind('行程.docx').label).toBe(CHAT.fileKindDoc)
    expect(fileKind('plan.pdf').label).toBe(CHAT.fileKindDoc)
    expect(fileKind('deck.pptx').label).toBe(CHAT.fileKindDeck)
  })

  it('falls back to the uppercase extension for unknown types', () => {
    expect(fileKind('archive.7z').label).toBe(CHAT.fileKindArchive)
    expect(fileKind('script.ps1').label).toBe('PS1')
    expect(fileKind('noext').label).toBe(CHAT.fileKindFile)
  })
})

describe('ChannelFileLink card', () => {
  it('renders an icon, file name, kind and a download affordance (no paperclip text)', () => {
    render(<ChannelFileLink name="黄山三日行程.docx" url="/v0/channels/media/x/y.docx" />)
    const card = host.querySelector('.channel-file-link')
    expect(card).not.toBeNull()
    expect(host.querySelector('.channel-file-name')?.textContent).toBe('黄山三日行程.docx')
    expect(host.querySelector('.channel-file-kind')?.textContent).toBe(CHAT.fileKindDoc)
    expect(card?.textContent).not.toContain(CHAT.clickToDownload)
  })
})

describe('ChannelFileLink delete', () => {
  it('deletes the file and its workspace copy', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 204 })
    vi.stubGlobal('fetch', fetchMock)
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    render(<ChannelFileLink name="行程.docx" url="/v0/channels/media/x/y.docx" />)
    const btn = host.querySelector('.channel-file-delete') as HTMLButtonElement
    expect(btn).not.toBeNull()
    await act(async () => {
      btn.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(fetchMock).toHaveBeenCalledWith(
      '/v0/channels/media/x/y.docx?name=%E8%A1%8C%E7%A8%8B.docx',
      expect.objectContaining({ method: 'DELETE' }),
    )
    expect(host.textContent).toContain(CHAT.attachmentDeleted)
  })

  it('hides the delete affordance for blob previews', () => {
    render(<ChannelFileLink name="n.pdf" url="blob:http://localhost/pdf" />)
    expect(host.querySelector('.channel-file-delete')).toBeNull()
  })
})
