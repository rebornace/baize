// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import { INBOX, inboxSecretHint } from '../strings'
import { InboxSettings } from './InboxSettings'

const sampleChannel = {
  id: 'alerts',
  agent_id: 'ticket-agent',
  enabled: true,
  description: 'ops',
  secret_hint: 'abcd',
}

async function renderPage(channels: api.InboxChannel[] = []) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  vi.spyOn(api, 'getInboxChannels').mockResolvedValue(channels)
  vi.spyOn(api, 'getUIConfig').mockResolvedValue({
    agent_id: 'ticket-agent',
    gate_enabled: true,
    supports_vision: false,
  })
  vi.spyOn(api, 'listSkills').mockResolvedValue({ skills: [] })
  await act(async () => {
    root.render(createElement(InboxSettings))
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

describe('InboxSettings UI', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows humanized title and empty state, not English Inbox heading', async () => {
    const { host, root } = await renderPage([])
    expect(host.textContent).toContain(INBOX.title)
    expect(host.textContent).toContain(INBOX.emptyTitle)
    expect(host.querySelector('h1')?.textContent).not.toBe('Inbox')
    root.unmount()
    host.remove()
  })

  it('asks ConfirmDialog before removing a channel; window.confirm unused', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm')
    const { host, root } = await renderPage([sampleChannel])

    expect(host.textContent).toContain('alerts')

    const removeBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(INBOX.remove),
    )
    expect(removeBtn).toBeTruthy()
    await act(async () => {
      removeBtn!.click()
    })

    expect(confirmSpy).not.toHaveBeenCalled()
    expect(host.textContent).toContain(INBOX.confirmRemoveTitle)
    expect(host.textContent).toContain('alerts')

    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
    })

    expect(host.textContent).not.toContain(INBOX.confirmRemoveTitle)
    const idInputs = [...host.querySelectorAll('input')].filter(
      (el) => (el as HTMLInputElement).value === 'alerts',
    )
    expect(idInputs).toHaveLength(0)

    root.unmount()
    host.remove()
  })

  it('rotate: ConfirmDialog then Modal with plaintext secret, no settings-drawer', async () => {
    const rotateSpy = vi.spyOn(api, 'rotateInboxSecret').mockResolvedValue({
      secret: 'plain-secret-once',
    })
    vi.spyOn(api, 'getInboxChannels')
      .mockResolvedValueOnce([sampleChannel])
      .mockResolvedValue([sampleChannel])

    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    vi.spyOn(api, 'getUIConfig').mockResolvedValue({
      agent_id: 'ticket-agent',
      gate_enabled: true,
      supports_vision: false,
    })
    vi.spyOn(api, 'listSkills').mockResolvedValue({ skills: [] })
    await act(async () => {
      root.render(createElement(InboxSettings))
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    const rotateBtn = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(INBOX.rotateSecret),
    )
    expect(rotateBtn).toBeTruthy()
    await act(async () => {
      rotateBtn!.click()
    })

    expect(host.textContent).toContain(INBOX.confirmRotateTitle)
    expect(rotateSpy).not.toHaveBeenCalled()

    await act(async () => {
      ;(host.querySelector('[data-testid="confirm-ok"]') as HTMLButtonElement).click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })

    expect(rotateSpy).toHaveBeenCalledWith('alerts')
    expect(host.textContent).toContain(INBOX.secretModalTitle)
    expect(host.textContent).toContain('plain-secret-once')
    expect(host.querySelector('.settings-drawer')).toBeNull()
    expect(host.querySelector('.settings-drawer-backdrop')).toBeNull()

    root.unmount()
    host.remove()
  })

  it('hides protocol jargon and shows human secret hint', async () => {
    const { host, root } = await renderPage([sampleChannel])
    expect(host.textContent).toContain(INBOX.description)
    expect(host.textContent).not.toMatch(/\bHMAC\b|给技术人员|KEY=VALUE/)
    expect(host.textContent).not.toMatch(/白泽|Baize/)
    expect(host.textContent).toContain(inboxSecretHint('abcd'))
    expect(host.textContent).not.toMatch(/secret\s*\u2026/i)
    expect(host.querySelector('details.settings-developer')).toBeNull()
    root.unmount()
    host.remove()
  })
})
