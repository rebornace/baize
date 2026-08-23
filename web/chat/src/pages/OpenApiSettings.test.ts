import { describe, expect, it } from 'vitest'
import {
  connectorToForm,
  openApiConnectorIds,
  validateOpenApiForm,
  type OpenApiFormState,
} from './OpenApiSettings'

describe('openApiConnectorIds', () => {
  it('includes spec and extra sources only', () => {
    const ids = openApiConnectorIds([
      { name: 'a', connector_id: 'o1', source: 'spec' },
      { name: 'b', connector_id: 'o1', source: 'spec' },
      { name: 'c', connector_id: 'o2', source: 'extra' },
      { name: 'd', connector_id: 'p1', source: 'plugin' },
      { name: 'e', connector_id: 'm1', source: 'mcp' },
    ])
    expect(ids).toEqual(['o1', 'o2'])
  })
})

describe('connectorToForm', () => {
  it('maps connector fields including callback url', () => {
    const form = connectorToForm({
      id: 'ticket-api',
      type: 'openapi',
      base_url: 'https://api.example.com',
      execution_callback_url: 'https://cb.example/hook',
      auth: {
        mode: 'static',
        static: { headers: { Authorization: 'Bearer x' } },
      },
      require_approval: ['create_ticket'],
      require_login: ['read'],
      import_format_detected: 'openapi3',
    })
    expect(form.id).toBe('ticket-api')
    expect(form.baseUrl).toBe('https://api.example.com')
    expect(form.executionCallbackUrl).toBe('https://cb.example/hook')
    expect(form.authHeadersText).toBe('Authorization=Bearer x')
    expect(form.requireApprovalText).toBe('create_ticket')
    expect(form.requireLoginText).toBe('read')
  })
})

describe('validateOpenApiForm', () => {
  const base: OpenApiFormState = {
    id: 'ticket-api',
    baseUrl: 'https://api.example.com',
    importFormat: 'auto',
    authMode: 'static',
    authHeadersText: 'Authorization=Bearer x',
    authPassthroughText: '',
    requireApprovalText: 'create_ticket',
    requireLoginText: '',
    executionCallbackUrl: 'https://cb.example/hook',
  }

  it('accepts valid create form with spec', () => {
    const result = validateOpenApiForm(base, { editing: false, hasNewSpec: true })
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.baseUrl).toBe('https://api.example.com')
      expect(result.executionCallbackUrl).toBe('https://cb.example/hook')
      expect(result.requireApproval).toEqual(['create_ticket'])
    }
  })

  it('requires spec on create', () => {
    expect(validateOpenApiForm(base, { editing: false, hasNewSpec: false }).ok).toBe(false)
  })

  it('accepts spec url on create', () => {
    expect(
      validateOpenApiForm(base, { editing: false, hasNewSpec: false, hasSpecUrl: true }).ok,
    ).toBe(true)
  })

  it('allows edit without new spec', () => {
    expect(validateOpenApiForm(base, { editing: true, hasNewSpec: false }).ok).toBe(true)
  })

  it('requires id and base_url', () => {
    expect(validateOpenApiForm({ ...base, id: '' }, { editing: true, hasNewSpec: false }).ok).toBe(
      false,
    )
    expect(
      validateOpenApiForm({ ...base, baseUrl: '' }, { editing: true, hasNewSpec: false }).ok,
    ).toBe(false)
  })
})
