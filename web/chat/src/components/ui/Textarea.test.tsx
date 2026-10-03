// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { Textarea } from './Textarea'

let host: HTMLDivElement
let root: ReturnType<typeof createRoot>
beforeEach(() => { host = document.createElement('div'); document.body.appendChild(host); root = createRoot(host) })
afterEach(() => { act(() => root.unmount()); host.remove() })

it('renders controlled value, disabled and invalid', async () => {
  const onChange = vi.fn()
  await act(async () => {
    root.render(<Textarea value="A=1" rows={3} disabled invalid onChange={onChange} />)
    await Promise.resolve()
  })
  const el = host.querySelector('textarea')!
  expect(el.value).toBe('A=1')
  expect(el.disabled).toBe(true)
  expect(el.rows).toBe(3)
  expect(el.className).toContain('invalid')
})
