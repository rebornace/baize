// @vitest-environment jsdom
import { act, useState } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
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
    vi.stubGlobal(
      'requestAnimationFrame',
      (cb: FrameRequestCallback) => window.setTimeout(() => cb(performance.now()), 16) as unknown as number,
    )
    vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id))
  })

  afterEach(() => {
    act(() => {
      root.unmount()
    })
    container.remove()
    vi.unstubAllGlobals()
  })

  it('snaps when inactive', () => {
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

  it('drains toward a growing target while active', async () => {
    function Grow() {
      const [t, setT] = useState('你好')
      const v = useSmoothStreamText(t, true)
      latest = v
      return (
        <button type="button" onClick={() => setT('你好，世界，这是更长的流式文本')}>
          grow
        </button>
      )
    }
    act(() => {
      root.render(<Grow />)
    })
    await act(async () => {
      await new Promise((r) => setTimeout(r, 80))
    })
    expect(latest.length).toBeGreaterThan(0)
    expect(latest.length).toBeLessThanOrEqual('你好，世界，这是更长的流式文本'.length)

    act(() => {
      container.querySelector('button')?.click()
    })
    await act(async () => {
      await new Promise((r) => setTimeout(r, 500))
    })
    expect(latest).toBe('你好，世界，这是更长的流式文本')
  })
})
