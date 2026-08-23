import { describe, expect, it } from 'vitest'
import {
  buildConnectorAuth,
  connectorToForm,
  formatKeyValueMap,
  parseKeyValueLines,
  parseLineList,
  pluginConnectorIds,
  validatePluginForm,
  type PluginFormState,
} from './PluginSettings'

describe('pluginConnectorIds', () => {
  it('dedupes plugin connector ids in order', () => {
    const ids = pluginConnectorIds([
      { name: 'a', connector_id: 'p1', source: 'plugin' },
      { name: 'b', connector_id: 'p1', source: 'plugin' },
      { name: 'c', connector_id: 'p2', source: 'plugin' },
      { name: 'd', connector_id: 'x', source: 'mcp' },
    ])
    expect(ids).toEqual(['p1', 'p2'])
  })
})

describe('parseKeyValueLines', () => {
  it('parses KEY=VALUE lines', () => {
    const parsed = parseKeyValueLines('A=1\nB=two')
    expect(parsed).toEqual({ ok: true, value: { A: '1', B: 'two' } })
  })

  it('rejects invalid lines', () => {
    const parsed = parseKeyValueLines('badline')
    expect(parsed.ok).toBe(false)
  })
})

describe('parseLineList', () => {
  it('trims and drops empty lines', () => {
    expect(parseLineList(' a \n\nb ')).toEqual(['a', 'b'])
  })
})

describe('formatKeyValueMap', () => {
  it('round-trips through parseKeyValueLines', () => {
    const text = formatKeyValueMap({ K: 'v', X: 'y' })
    expect(parseKeyValueLines(text)).toEqual({ ok: true, value: { K: 'v', X: 'y' } })
  })
})

describe('buildConnectorAuth', () => {
  it('builds static auth from headers text', () => {
    const result = buildConnectorAuth('static', 'Authorization=Bearer x', '')
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.auth).toEqual({
        mode: 'static',
        static: { headers: { Authorization: 'Bearer x' } },
      })
    }
  })

  it('builds passthrough auth from line list', () => {
    const result = buildConnectorAuth('passthrough', '', 'Authorization\nX-Trace')
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.auth).toEqual({
        mode: 'passthrough',
        passthrough: { headers: ['Authorization', 'X-Trace'] },
      })
    }
  })

  it('builds vault_ref auth from headers text', () => {
    const result = buildConnectorAuth('vault_ref', 'Authorization=vault:prod/key', '')
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.auth).toEqual({
        mode: 'vault_ref',
        vault_ref: { headers: { Authorization: 'vault:prod/key' } },
      })
    }
  })
})

describe('connectorToForm', () => {
  it('maps connector auth back to form fields', () => {
    const form = connectorToForm({
      id: 'sidecar',
      type: 'http',
      base_url: 'http://127.0.0.1:19090',
      auth: {
        mode: 'static',
        static: { headers: { Authorization: 'Bearer ${KEY}' } },
      },
      require_approval: ['create_ticket'],
      require_login: ['read'],
    })
    expect(form.id).toBe('sidecar')
    expect(form.baseUrl).toBe('http://127.0.0.1:19090')
    expect(form.authMode).toBe('static')
    expect(form.authHeadersText).toBe('Authorization=Bearer ${KEY}')
    expect(form.requireApprovalText).toBe('create_ticket')
    expect(form.requireLoginText).toBe('read')
  })
})

describe('validatePluginForm', () => {
  const base: PluginFormState = {
    id: 'legacy-sidecar',
    baseUrl: 'http://127.0.0.1:19090',
    authMode: 'static',
    authHeadersText: 'Authorization=Bearer x',
    authPassthroughText: '',
    requireApprovalText: 'create_ticket',
    requireLoginText: '',
  }

  it('accepts valid form', () => {
    const result = validatePluginForm(base)
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.baseUrl).toBe('http://127.0.0.1:19090')
      expect(result.auth.mode).toBe('static')
      expect(result.requireApproval).toEqual(['create_ticket'])
    }
  })

  it('requires id and base_url', () => {
    expect(validatePluginForm({ ...base, id: '' }).ok).toBe(false)
    expect(validatePluginForm({ ...base, baseUrl: '' }).ok).toBe(false)
  })
})
