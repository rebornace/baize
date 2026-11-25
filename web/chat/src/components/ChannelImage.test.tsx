// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GateContext } from '../gateContext'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { CHAT } from '../strings'
import { ChannelImage } from './ChannelImage'

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

function render(url: string, name = '', document = false) {
  act(() => {
    createRoot(host).render(
      <GateContext.Provider value={{ role: 'operator', gateEnabled: false, operatorId: 'op' }}>
        <ChannelImage url={url} name={name} document={document} />
      </GateContext.Provider>,
    )
  })
}

describe('ChannelImage attachment delete', () => {
  it('deletes a persisted image and replaces it with a tombstone', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 204 })
    vi.stubGlobal('fetch', fetchMock)
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    render('/v0/channels/media/c/a.png?name=shot.png', 'shot.png')
    const btn = host.querySelector('.channel-attachment-delete') as HTMLButtonElement
    expect(btn).not.toBeNull()
    await act(async () => {
      btn.click()
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(fetchMock).toHaveBeenCalledWith(
      '/v0/channels/media/c/a.png?name=shot.png',
      expect.objectContaining({ method: 'DELETE' }),
    )
    expect(host.textContent).toContain(CHAT.attachmentDeleted)
    expect(host.querySelector('img.channel-image')).toBeNull()
  })

  it('keeps the image when the confirm is cancelled', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    vi.spyOn(window, 'confirm').mockReturnValue(false)

    render('/v0/channels/media/c/a.png', 'a.png')
    const btn = host.querySelector('.channel-attachment-delete') as HTMLButtonElement
    await act(async () => {
      btn.click()
      await Promise.resolve()
    })
    expect(fetchMock).not.toHaveBeenCalled()
    expect(host.querySelector('img.channel-image')).not.toBeNull()
  })

  it('shows a failure note and keeps the image when delete fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 500 }))
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    render('/v0/channels/media/c/a.png', 'a.png')
    const btn = host.querySelector('.channel-attachment-delete') as HTMLButtonElement
    await act(async () => {
      btn.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(host.textContent).toContain(CHAT.deleteAttachmentFailed)
    expect(host.querySelector('img.channel-image')).not.toBeNull()
  })

  it('hides the delete affordance for local optimistic previews', () => {
    render('blob:http://localhost/img')
    expect(host.querySelector('.channel-attachment-delete')).toBeNull()
  })

  it('captions document-derived images and hides delete', () => {
    render(
      '/v0/channels/media/c/p.jpg?name=scan.pdf%20%C2%B7%20page-1&from=doc',
      'scan.pdf · page-1',
      true,
    )
    expect(host.textContent).toContain(CHAT.documentImageCaption('scan.pdf · page-1'))
    expect(host.querySelector('.channel-attachment-delete')).toBeNull()
  })
})
