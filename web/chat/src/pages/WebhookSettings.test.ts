import { describe, expect, it } from 'vitest'
import { configToForm, validateWebhookForm, type WebhookFormState } from './WebhookSettings'

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
