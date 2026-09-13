import { describe, expect, it } from 'vitest'
import {
  configToForm,
  deliverySummary,
  formatDeliveryStatus,
  validateWebhookForm,
  type WebhookFormState,
} from './WebhookSettings'
import type { EventsWebhookDelivery } from '../api'
import { WEBHOOKS } from '../strings'

describe('configToForm', () => {
  it('maps url and headers to form state', () => {
    expect(
      configToForm({
        url: 'https://example.com/hook',
        headers: { Authorization: 'Bearer x' },
      }),
    ).toEqual({
      url: 'https://example.com/hook',
      headersText: 'Authorization=Bearer x',
    })
  })

  it('handles empty config', () => {
    expect(configToForm({ url: '', headers: {} })).toEqual({
      url: '',
      headersText: '',
    })
  })
})

describe('validateWebhookForm', () => {
  const base: WebhookFormState = {
    url: 'https://example.com/hook',
    headersText: 'X-Test=1',
  }

  it('accepts valid form', () => {
    const result = validateWebhookForm(base)
    expect(result).toEqual({
      ok: true,
      config: {
        url: 'https://example.com/hook',
        headers: { 'X-Test': '1' },
      },
    })
  })

  it('trims url', () => {
    const result = validateWebhookForm({ ...base, url: '  https://a.test  ' })
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.config.url).toBe('https://a.test')
    }
  })

  it('allows empty url and headers', () => {
    const result = validateWebhookForm({ url: '', headersText: '' })
    expect(result).toEqual({
      ok: true,
      config: { url: '', headers: {} },
    })
  })

  it('rejects invalid header lines', () => {
    const result = validateWebhookForm({ ...base, headersText: 'badline' })
    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.message).toBe(WEBHOOKS.errBadHeaderLine)
    }
  })
})

describe('formatDeliveryStatus', () => {
  it('maps known statuses', () => {
    expect(formatDeliveryStatus('dead')).toBe(WEBHOOKS.statusDead)
    expect(formatDeliveryStatus('pending')).toBe(WEBHOOKS.statusPending)
    expect(formatDeliveryStatus('delivered')).toBe(WEBHOOKS.statusDelivered)
  })

  it('uses outcome-oriented labels without protocol jargon in main copy keys', () => {
    expect(WEBHOOKS.statusDead).toBe('已停止（多次失败）')
    expect(WEBHOOKS.description).toBe('有消息或任务结束时，自动通知你填的网址。')
    expect(WEBHOOKS.description).not.toMatch(/引擎|HMAC|5xx|KEY=VALUE|白泽|Baize|不是|企业统一执行/i)
    expect(WEBHOOKS.deliveriesHint).not.toMatch(/5xx|429|4xx|死信/)
    expect(WEBHOOKS.headersHint).not.toMatch(/KEY=VALUE/)
    expect(WEBHOOKS.errBadHeaderLine).not.toMatch(/KEY=VALUE/)
  })

  it('passes through unknown status', () => {
    expect(formatDeliveryStatus('custom')).toBe('custom')
  })
})

describe('deliverySummary', () => {
  const base: EventsWebhookDelivery = {
    id: 'd1',
    run_id: 'run_x',
    kind: 'event',
    event_index: 2,
    status: 'dead',
    attempt: 3,
    max_attempts: 5,
    next_retry_at: '2026-01-01T00:00:00Z',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }

  it('formats event delivery', () => {
    expect(deliverySummary(base)).toBe('run_x · event#2 · 3/5')
  })

  it('includes last error when present', () => {
    expect(deliverySummary({ ...base, last_error: 'HTTP 503' })).toContain('HTTP 503')
  })

  it('formats ended delivery', () => {
    expect(deliverySummary({ ...base, kind: 'ended', event_index: -1 })).toContain('run.ended')
  })
})
