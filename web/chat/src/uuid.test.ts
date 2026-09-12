import { describe, expect, it } from 'vitest'
import { uuid } from './uuid'

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/

describe('uuid', () => {
  it('returns a canonical v4 string when crypto.randomUUID exists', () => {
    const id = uuid()
    expect(id).toMatch(UUID_RE)
    expect(id[14]).toBe('4')
    expect(['8', '9', 'a', 'b']).toContain(id[19])
  })

  it('produces unique values', () => {
    const seen = new Set<string>()
    for (let i = 0; i < 1000; i++) seen.add(uuid())
    expect(seen.size).toBe(1000)
  })

  it('falls back to getRandomValues in insecure contexts (no randomUUID)', () => {
    // Simulate http://192.168.x.x: an insecure context where randomUUID is
    // undefined but getRandomValues still exists. This path must not throw and
    // must still yield a valid v4 UUID.
    const cryptoObj = globalThis.crypto
    const descriptor = Object.getOwnPropertyDescriptor(cryptoObj, 'randomUUID')
    Object.defineProperty(cryptoObj, 'randomUUID', {
      configurable: true,
      writable: true,
      value: undefined,
    })
    try {
      expect(typeof crypto.randomUUID).toBe('undefined')
      const id = uuid()
      expect(id).toMatch(UUID_RE)
      expect(id[14]).toBe('4')
      expect(['8', '9', 'a', 'b']).toContain(id[19])
      expect(typeof cryptoObj.getRandomValues).toBe('function')
    } finally {
      if (descriptor) {
        Object.defineProperty(cryptoObj, 'randomUUID', descriptor)
      }
    }
  })
})
