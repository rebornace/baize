// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from '../api'
import { WEBHOOKS } from '../strings'
import { WebhookSettings } from './WebhookSettings'

async function renderPage() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  vi.spyOn(api, 'getEventsWebhook').mockResolvedValue({ url: '', headers: {} })
  vi.spyOn(api, 'getEventsWebhookDeliveries').mockResolvedValue([])
  await act(async () => {
    root.render(createElement(WebhookSettings))
    await new Promise((r) => setTimeout(r, 0))
    await new Promise((r) => setTimeout(r, 0))
  })
  return { host, root }
}

describe('WebhookSettings UI', () => {
  afterEach(() => vi.restoreAllMocks())

  it('shows humanized title and empty-url hint, not English Webhook heading', async () => {
    const { host, root } = await renderPage()
    expect(host.textContent).toContain(WEBHOOKS.title)
    expect(host.textContent).toContain(WEBHOOKS.urlHint)
    expect(host.querySelector('h1')?.textContent).not.toBe('Webhook')
    root.unmount()
    host.remove()
  })

  it('save success uses toast path (putEventsWebhook called)', async () => {
    const put = vi.spyOn(api, 'putEventsWebhook').mockResolvedValue({
      url: 'https://example.com/h',
      headers: {},
    })
    const { host, root } = await renderPage()
    const url = host.querySelector('input') as HTMLInputElement
    await act(async () => {
      const nativeInputValueSetter = Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        'value',
      )!.set!
      nativeInputValueSetter.call(url, 'https://example.com/h')
      url.dispatchEvent(new Event('input', { bubbles: true }))
    })
    const save = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes(WEBHOOKS.save),
    )
    await act(async () => {
      save!.click()
      await new Promise((r) => setTimeout(r, 0))
      await new Promise((r) => setTimeout(r, 0))
    })
    expect(put).toHaveBeenCalled()
    expect(host.textContent).toContain(WEBHOOKS.toastSaved)
    root.unmount()
    host.remove()
  })
})
