import { describe, expect, it } from 'vitest'
import { conversationExportFilename } from './conversationExport'

describe('conversationExportFilename', () => {
  it('keeps safe ids and replaces path separators', () => {
    expect(conversationExportFilename('conv_123-abc')).toBe('baize-conv_123-abc.md')
  })

  it('sanitizes channel conversation ids', () => {
    expect(conversationExportFilename('weixin:acc/1:peer 2')).toBe('baize-weixin-acc-1-peer-2.md')
  })
})
