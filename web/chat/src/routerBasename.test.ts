import { afterEach, describe, expect, it, vi } from 'vitest'
import { routerBasename, uiHomeURL } from './routerBasename'

describe('routerBasename', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('defaults to /ui for embedded console', () => {
    vi.stubEnv('VITE_ROUTER_BASENAME', '')
    expect(routerBasename()).toBe('/ui')
    expect(uiHomeURL()).toBe('/ui/')
  })

  it('uses empty basename for Capacitor root shell', () => {
    vi.stubEnv('VITE_ROUTER_BASENAME', '/')
    expect(routerBasename()).toBe('')
    expect(uiHomeURL()).toBe('/')
  })
})
