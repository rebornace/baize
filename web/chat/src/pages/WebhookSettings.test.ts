import { describe, expect, it } from 'vitest'
import {
  configToForm,
  deliverySummary,
  formatDeliveryStatus,
  validateWebhookForm,
  type WebhookFormState,
} from './WebhookSettings'
import type { EventsWebhookDelivery } from '../api'

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
      expect(result.message).toContain('无效键值行')
    }
  })
})

describe('formatDeliveryStatus', () => {
  it('maps known statuses', () => {
    expect(formatDeliveryStatus('dead')).toBe('死信')
    expect(formatDeliveryStatus('pending')).toBe('待投递')
    expect(formatDeliveryStatus('delivered')).toBe('已投递')
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
