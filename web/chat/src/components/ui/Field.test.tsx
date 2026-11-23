// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { Field } from './Field'
import { Input } from './Input'
import { Select } from './Select'

let host: HTMLDivElement
let root: ReturnType<typeof createRoot>
beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
})

function render(el: React.ReactNode) {
  act(() => {
    root.render(el)
  })
}

describe('Field a11y wiring', () => {
  it('associates label/control via the given htmlFor', () => {
    render(<Field label="邮箱" htmlFor="email"><Input type="text" /></Field>)
    const input = host.querySelector('input')!
    expect(host.querySelector('label')!.getAttribute('for')).toBe('email')
    expect(input.id).toBe('email')
  })

  it('marks the control invalid and references the error node on error', () => {
    render(
      <Field label="邮箱" htmlFor="email" error="必填项">
        <Input type="text" />
      </Field>,
    )
    const input = host.querySelector('input')!
    expect(input.getAttribute('aria-invalid')).toBe('true')
    expect(input.getAttribute('aria-describedby')).toBe('email-error')
    expect(host.querySelector('#email-error')!.textContent).toBe('必填项')
    expect(host.querySelector('#email-hint')).toBeNull()
  })

  it('references hint when no error, and omits describedby when neither', () => {
    render(<Field label="路径" htmlFor="p" hint="默认 ./data/baize.db"><Input /></Field>)
    const input = host.querySelector('input')!
    expect(input.getAttribute('aria-invalid')).toBeNull()
    expect(input.getAttribute('aria-describedby')).toBe('p-hint')

    render(<Field label="x" htmlFor="x"><Select><option value="a">A</option></Select></Field>)
    const select = host.querySelector('select')!
    expect(select.getAttribute('aria-describedby')).toBeNull()
  })
})
