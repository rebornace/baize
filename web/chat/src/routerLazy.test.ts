import { describe, expect, it, vi } from 'vitest'
import { ROUTE_RELOAD_KEY, loadRouteModule } from './routerLazy'

function memoryStorage() {
  const data = new Map<string, string>()
  return {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => {
      data.set(key, value)
    },
  }
}

describe('loadRouteModule', () => {
  it('returns the module when the chunk loads', async () => {
    const mod = { default: 'ok' }
    await expect(
      loadRouteModule(() => Promise.resolve(mod), vi.fn(), () => 1000, memoryStorage()),
    ).resolves.toBe(mod)
  })

  it('reloads once on the first failure', async () => {
    const storage = memoryStorage()
    const reload = vi.fn()
    await expect(
      loadRouteModule(() => Promise.reject(new Error('chunk 404')), reload, () => 1000, storage),
    ).rejects.toThrow('chunk 404')
    expect(reload).toHaveBeenCalledTimes(1)
    expect(storage.getItem(ROUTE_RELOAD_KEY)).toBe('1000')
  })

  it('does not reload again within the guard window', async () => {
    const storage = memoryStorage()
    storage.setItem(ROUTE_RELOAD_KEY, '1000')
    const reload = vi.fn()
    await expect(
      loadRouteModule(() => Promise.reject(new Error('still 404')), reload, () => 5000, storage),
    ).rejects.toThrow('still 404')
    expect(reload).not.toHaveBeenCalled()
  })

  it('reloads again after the guard window', async () => {
    const storage = memoryStorage()
    storage.setItem(ROUTE_RELOAD_KEY, '1000')
    const reload = vi.fn()
    await expect(
      loadRouteModule(() => Promise.reject(new Error('after window')), reload, () => 40000, storage),
    ).rejects.toThrow('after window')
    expect(reload).toHaveBeenCalledTimes(1)
    expect(storage.getItem(ROUTE_RELOAD_KEY)).toBe('40000')
  })

  it('still reloads when session storage is unavailable', async () => {
    const reload = vi.fn()
    await expect(
      loadRouteModule(() => Promise.reject(new Error('no storage')), reload, () => 1000, null),
    ).rejects.toThrow('no storage')
    expect(reload).toHaveBeenCalledTimes(1)
  })
})
