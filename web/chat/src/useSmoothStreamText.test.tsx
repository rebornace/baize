// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { describe, expect, it, beforeEach, afterEach } from 'vitest'
import { useSmoothStreamText } from './useSmoothStreamText'

function HookProbe({
  target,
  active,
  onValue,
}: {
  target: string
  active: boolean
  onValue: (v: string) => void
}) {
  const v = useSmoothStreamText(target, active)
  onValue(v)
  return null
}

describe('useSmoothStreamText', () => {
  let root: Root
  let container: HTMLDivElement
  let latest = ''

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    latest = ''
  })

  afterEach(() => {
    act(() => {
      root.unmount()
    })
    container.remove()
  })

  it('returns the full target when inactive', () => {
    act(() => {
      root.render(
        <HookProbe
          target="完整历史"
          active={false}
          onValue={(v) => {
            latest = v
          }}
        />,
      )
    })
    expect(latest).toBe('完整历史')
  })

  it('returns the server snapshot immediately while active', () => {
    act(() => {
      root.render(
        <HookProbe
          target="你好，世界，这是流式文本"
          active={true}
          onValue={(v) => {
            latest = v
          }}
        />,
      )
    })
    expect(latest).toBe('你好，世界，这是流式文本')
  })
})
