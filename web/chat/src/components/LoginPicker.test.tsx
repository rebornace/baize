// @vitest-environment jsdom
import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LoginEntry } from '../api'
import { LOGIN_AT } from '../strings'
import { LoginPicker } from './LoginPicker'

const CRM: LoginEntry = {
  id: 'crm/login',
  connector_id: 'crm',
  connector_title: 'CRM',
  connector_type: 'openapi',
  tool_name: 'login',
  title: '登录 · CRM',
  logged_in: false,
  required: [],
}

const HR: LoginEntry = {
  id: 'hr/login',
  connector_id: 'hr',
  connector_title: 'HR',
  connector_type: 'openapi',
  tool_name: 'login',
  title: '登录 · HR',
  logged_in: false,
  required: [],
}

describe('LoginPicker connector isolation', () => {
  let host: HTMLDivElement
  let root: Root

  beforeEach(() => {
    ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
      true
    host = document.createElement('div')
    document.body.appendChild(host)
  })

  afterEach(() => {
    act(() => {
      root?.unmount()
    })
    host.remove()
    vi.restoreAllMocks()
  })

  function render(el: ReactNode) {
    act(() => {
      root = createRoot(host)
      root.render(el)
    })
  }

  it('with connector-filtered entries only shows that connector', () => {
    render(
      <LoginPicker
        open
        entries={[CRM]}
        onClose={() => {}}
        onPick={() => {}}
      />,
    )
    expect(host.textContent).toContain('登录 · CRM')
    expect(host.textContent).not.toContain('登录 · HR')
  })

  it('without connector_id shows emptyMessage and no other-connector rows', () => {
    // Parent must pass [] when connector id is missing — never [CRM, HR].
    render(
      <LoginPicker
        open
        entries={[]}
        emptyMessage={LOGIN_AT.pickerNoConnector}
        onClose={() => {}}
        onPick={() => {}}
      />,
    )
    expect(host.textContent).toContain(LOGIN_AT.pickerNoConnector)
    expect(host.textContent).not.toContain('登录 · CRM')
    expect(host.textContent).not.toContain('登录 · HR')
    expect(host.textContent).not.toContain(HR.title)
  })
})
