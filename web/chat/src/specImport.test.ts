import { describe, expect, it } from 'vitest'
import { detectImportFormat, detectedFormatHint, importFormatLabel } from './specImport'

const openapi3Fixture = JSON.stringify({
  openapi: '3.0.3',
  info: { title: 'Minimal API', version: '1.0.0' },
  paths: { '/items': { get: { operationId: 'list_items', responses: { '200': { description: 'ok' } } } } },
})

const swagger2Fixture = JSON.stringify({
  swagger: '2.0',
  info: { title: 'Minimal API', version: '1.0.0' },
  paths: { '/items': { get: { operationId: 'list_items', responses: { '200': { description: 'ok' } } } } },
})

const postmanFixture = JSON.stringify({
  info: {
    name: 'Minimal Collection',
    schema: 'https://schema.getpostman.com/json/collection/v2.1.0/collection.json',
  },
  item: [{ name: 'List Items', request: { method: 'GET', url: { raw: '{{baseUrl}}/items' } } }],
})

describe('detectImportFormat', () => {
  it('detects openapi3', () => {
    expect(detectImportFormat(openapi3Fixture)).toBe('openapi3')
  })

  it('detects swagger2', () => {
    expect(detectImportFormat(swagger2Fixture)).toBe('swagger2')
  })

  it('detects postman', () => {
    expect(detectImportFormat(postmanFixture)).toBe('postman')
  })

  it('returns null for invalid json', () => {
    expect(detectImportFormat('not json')).toBeNull()
  })

  it('returns null for unrecognized root', () => {
    expect(detectImportFormat('{"foo":1}')).toBeNull()
  })
})

describe('importFormatLabel', () => {
  it('maps known formats', () => {
    expect(importFormatLabel('openapi3')).toBe('OpenAPI 3')
    expect(importFormatLabel('swagger2')).toBe('Swagger 2')
    expect(importFormatLabel('postman')).toBe('Postman Collection')
    expect(importFormatLabel('auto')).toBe('自动识别')
  })
})

describe('detectedFormatHint', () => {
  it('describes swagger conversion', () => {
    expect(detectedFormatHint('swagger2')).toContain('Swagger 2')
  })

  it('handles unknown detection', () => {
    expect(detectedFormatHint(null)).toContain('未能自动识别')
  })
})
