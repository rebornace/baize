import { describe, expect, it } from 'vitest'
import { conversationExportFilename, conversationExportMime } from './conversationExport'

describe('conversationExportFilename', () => {
  it('keeps safe ids and replaces path separators', () => {
    expect(conversationExportFilename('conv_123-abc')).toBe('baize-conv_123-abc.md')
  })

  it('sanitizes channel conversation ids', () => {
    expect(conversationExportFilename('weixin:acc/1:peer 2')).toBe('baize-weixin-acc-1-peer-2.md')
  })

  it('uses the selected format extension', () => {
    expect(conversationExportFilename('conv_1', 'json')).toBe('baize-conv_1.json')
    expect(conversationExportFilename('conv_1', 'html')).toBe('baize-conv_1.html')
    expect(conversationExportFilename('conv_1', 'markdown')).toBe('baize-conv_1.md')
  })
})

describe('conversationExportMime', () => {
  it('maps each format to its download MIME', () => {
    expect(conversationExportMime('markdown')).toContain('text/markdown')
    expect(conversationExportMime('json')).toContain('application/json')
    expect(conversationExportMime('html')).toContain('text/html')
  })
})
