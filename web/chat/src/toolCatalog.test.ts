import { describe, expect, it } from 'vitest'
import { canDeleteCatalogTool } from './toolCatalog'

describe('canDeleteCatalogTool', () => {
  it('only extra', () => {
    expect(canDeleteCatalogTool('extra')).toBe(true)
    expect(canDeleteCatalogTool('spec')).toBe(false)
    expect(canDeleteCatalogTool('plugin')).toBe(false)
  })
})
