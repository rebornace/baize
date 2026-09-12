// @vitest-environment jsdom
import { type ReactNode } from 'react'
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { UserBubble } from './UserBubble'
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

describe('UserBubble attachments', () => {
  it('renders an inline image + download card from persisted media markers', () => {
    render(
      <UserBubble
        content={
          '看这两个\n' +
          '![图片](/v0/channels/media/c/a.png)\n' +
          '[file:行程.docx](/v0/channels/media/c/b.docx)'
        }
      />,
    )
    const img = host.querySelector<HTMLImageElement>('img.channel-image')
    expect(img).not.toBeNull()
    expect(img?.getAttribute('src')).toBe('/v0/channels/media/c/a.png')
    expect(host.querySelector('.channel-file-link')).not.toBeNull()
    expect(host.querySelector('.channel-file-name')?.textContent).toBe('行程.docx')
    // Typed text shows once; no redundant attachment note; no literal markers.
    expect(host.textContent?.match(/看这两个/g)).toHaveLength(1)
    expect(host.textContent).not.toContain('（附件：')
    expect(host.textContent).not.toContain('![图片]')
    expect(host.textContent).not.toContain('[file:')
  })

  it('renders optimistic blob: previews inline (just-sent web upload)', () => {
    render(
      <UserBubble
        content={
          '刚发的\n' +
          '![图片](blob:http://localhost/img)\n' +
          '[file:n.pdf](blob:http://localhost/pdf)'
        }
      />,
    )
    const img = host.querySelector<HTMLImageElement>('img.channel-image')
    expect(img?.getAttribute('src')).toBe('blob:http://localhost/img')
    const card = host.querySelector('.channel-file-link')
    expect(card).not.toBeNull()
    expect(host.querySelector('.channel-file-name')?.textContent).toBe('n.pdf')
    expect(host.textContent).not.toContain('（附件：')
  })

  it('renders attachment-only bubbles with no leftover text node', () => {
    render(<UserBubble content={'![图片](/v0/channels/media/c/a.jpg)'} />)
    expect(host.querySelector('img.channel-image')).not.toBeNull()
  })
})
