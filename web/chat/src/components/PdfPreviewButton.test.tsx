// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GateContext } from '../gateContext'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { PdfPreviewButton } from './PdfPreviewButton'

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

function render(url: string, name: string) {
  act(() => {
    createRoot(host).render(
      <GateContext.Provider value={{ role: 'operator', gateEnabled: false, operatorId: 'op' }}>
        <PdfPreviewButton url={url} name={name} />
      </GateContext.Provider>,
    )
  })
}

describe('PdfPreviewButton', () => {
  it('opens the PDF in a full-screen iframe', async () => {
    render('/v0/channels/media/c/a.pdf?name=a.pdf', 'a.pdf')
    const trigger = host.querySelector('.channel-file-preview') as HTMLButtonElement
    expect(trigger).not.toBeNull()
    await act(async () => {
      trigger.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(host.querySelector('.media-lightbox')).not.toBeNull()
    const frame = host.querySelector('.pdf-preview-frame') as HTMLIFrameElement
    expect(frame).not.toBeNull()
    expect(frame.src).toContain('/v0/channels/media/c/a.pdf')
  })

  it('fetches with auth when the gate is on', async () => {
    const arrayBuffer = new ArrayBuffer(4)
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      arrayBuffer: async () => arrayBuffer,
    })
    vi.stubGlobal('fetch', fetchMock)
    const createObjectURL = vi.fn().mockReturnValue('blob:preview')
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL: vi.fn() })
    act(() => {
      createRoot(host).render(
        <GateContext.Provider value={{ role: 'operator', gateEnabled: true, operatorId: 'op' }}>
          <PdfPreviewButton url="/v0/channels/media/c/a.pdf" name="a.pdf" />
        </GateContext.Provider>,
      )
    })
    const trigger = host.querySelector('.channel-file-preview') as HTMLButtonElement
    await act(async () => {
      trigger.click()
      await Promise.resolve()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(fetchMock).toHaveBeenCalled()
    const frame = host.querySelector('.pdf-preview-frame') as HTMLIFrameElement
    expect(frame?.src).toContain('blob:preview')
  })
})
