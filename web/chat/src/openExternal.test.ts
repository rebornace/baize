// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { openExternal } from './openExternal'

describe('openExternal', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('uses window.open when Capacitor is unavailable', async () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    await openExternal('https://login.example/hosted')
    expect(open).toHaveBeenCalledWith('https://login.example/hosted', '_blank', 'noopener,noreferrer')
  })

  it('no-ops on empty url', async () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    await openExternal('  ')
    expect(open).not.toHaveBeenCalled()
  })
})
