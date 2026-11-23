// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiOrigin, apiURL, installApiOrigin } from './origin'

describe('api origin helpers', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('apiURL is identity when VITE_API_ORIGIN is empty', () => {
    vi.stubEnv('VITE_API_ORIGIN', '')
    expect(apiOrigin()).toBe('')
    expect(apiURL('/v0/me')).toBe('/v0/me')
  })

  it('apiURL prefixes origin and strips trailing slash', () => {
    vi.stubEnv('VITE_API_ORIGIN', 'https://baize.example/')
    expect(apiOrigin()).toBe('https://baize.example')
    expect(apiURL('/v0/me')).toBe('https://baize.example/v0/me')
  })

  it('installApiOrigin rewrites root-relative fetch', async () => {
    vi.stubEnv('VITE_API_ORIGIN', 'https://baize.example')
    const calls: string[] = []
    const native = vi.fn(async (input: RequestInfo | URL) => {
      calls.push(typeof input === 'string' ? input : String(input))
      return new Response('{}', { status: 200 })
    })
    const prev = window.fetch
    window.fetch = native as unknown as typeof fetch
    try {
      installApiOrigin()
      await fetch('/v0/me')
      expect(calls).toEqual(['https://baize.example/v0/me'])
    } finally {
      window.fetch = prev
    }
  })
})
