// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as alertsApi from '../api/settings/alerts'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { AlertChannelsCard } from './AlertChannelsCard'

let host: HTMLDivElement
beforeEach(() => {
  ;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true
  setPack(zhPack)
  host = document.createElement('div')
  document.body.appendChild(host)
})
afterEach(() => {
  host.remove()
  vi.restoreAllMocks()
  setPack(zhPack)
})

describe('AlertChannelsCard', () => {
  it('loads channels and sends a test notification', async () => {
    vi.spyOn(alertsApi, 'getAlertChannels').mockResolvedValue({
      max: 20,
      items: [
        {
          id: 'a1',
          name: 'Ops',
          type: 'generic',
          url: 'https://example.com/hook',
          enabled: true,
        },
      ],
    })
    vi.spyOn(alertsApi, 'getSMTPSettings').mockResolvedValue({
      host: '',
      port: 587,
      username: '',
      from: '',
      security: 'starttls',
      has_password: false,
    })
    const testSpy = vi.spyOn(alertsApi, 'testAlertChannel').mockResolvedValue({ status: 'sent' })
    await act(async () => {
      createRoot(host).render(<AlertChannelsCard />)
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(host.querySelector('[data-testid="alert-channels"]')).not.toBeNull()
    expect(host.textContent).toContain('告警通知通道')
    const testButton = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('测试'),
    )
    await act(async () => {
      testButton?.click()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(testSpy).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'a1', type: 'generic' }),
    )
    expect(host.textContent).toContain('测试通知已发送')
  })
})
