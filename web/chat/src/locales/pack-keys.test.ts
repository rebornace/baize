import { describe, expect, it } from 'vitest'
import { enPack } from './en'
import { zhPack } from './zh'

function keyPaths(value: unknown, prefix = ''): string[] {
  if (value === null || typeof value !== 'object') return [prefix]
  if (typeof value === 'function') return [prefix]
  const keys = Object.keys(value as object).sort()
  if (keys.length === 0) return [prefix]
  return keys.flatMap((k) => {
    const next = prefix ? `${prefix}.${k}` : k
    const child = (value as Record<string, unknown>)[k]
    if (typeof child === 'function') return [next]
    if (child !== null && typeof child === 'object') return keyPaths(child, next)
    return [next]
  })
}

describe('locale pack key parity', () => {
  it('en and zh expose the same nested key set', () => {
    expect(keyPaths(enPack).sort()).toEqual(keyPaths(zhPack).sort())
  })
})
