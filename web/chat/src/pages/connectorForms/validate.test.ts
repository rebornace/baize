import { describe, expect, it } from 'vitest'
import { validateConnection, CONNECTOR_ID_RE } from './validate'

describe('CONNECTOR_ID_RE', () => {
  it('accepts valid ids', () => {
    expect(CONNECTOR_ID_RE.test('ticket-api')).toBe(true)
    expect(CONNECTOR_ID_RE.test('a')).toBe(true)
    expect(CONNECTOR_ID_RE.test('crm_2')).toBe(true)
  })
  it('rejects invalid ids', () => {
    expect(CONNECTOR_ID_RE.test('Ticket')).toBe(false)
    expect(CONNECTOR_ID_RE.test('2start')).toBe(false)
    expect(CONNECTOR_ID_RE.test('has space')).toBe(false)
    expect(CONNECTOR_ID_RE.test('')).toBe(false)
  })
})

describe('validateConnection (plugin: no spec)', () => {
  it('requires id and base url', () => {
    const r = validateConnection({ kind: 'plugin', id: '  ', baseUrl: '', hasSpec: false })
    expect(r.ok).toBe(false)
    if (!r.ok) {
      expect(r.fieldErrors.id).toBeTruthy()
      expect(r.fieldErrors.baseUrl).toBeTruthy()
    }
  })
  it('accepts a valid plugin form', () => {
    const r = validateConnection({ kind: 'plugin', id: 'p1', baseUrl: ' http://127.0.0.1:19090 ', hasSpec: false })
    expect(r.ok).toBe(true)
    if (r.ok) expect(r.baseUrl).toBe('http://127.0.0.1:19090')
  })
  it('rejects non-http base url', () => {
    const r = validateConnection({ kind: 'plugin', id: 'p1', baseUrl: 'ftp://x', hasSpec: false })
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.fieldErrors.baseUrl).toBeTruthy()
  })
})

describe('validateConnection (openapi: needs spec on create)', () => {
  it('requires a spec on create', () => {
    const r = validateConnection({ kind: 'openapi', id: 'o1', baseUrl: 'https://x', hasSpec: false })
    expect(r.ok).toBe(false)
    if (!r.ok) expect(r.fieldErrors.spec).toBeTruthy()
  })
  it('accepts when a spec file or url is present', () => {
    const r = validateConnection({ kind: 'openapi', id: 'o1', baseUrl: 'https://x', hasSpec: true })
    expect(r.ok).toBe(true)
  })
  it('editing an existing connector allows omitting a new spec', () => {
    const r = validateConnection({ kind: 'openapi', id: 'o1', baseUrl: 'https://x', hasSpec: false, editing: true })
    expect(r.ok).toBe(true)
  })
})
