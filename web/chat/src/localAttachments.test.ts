import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { buildLocalPreview, extractBlobURLs } from './localAttachments'

describe('buildLocalPreview', () => {
  let seq = 0
  beforeEach(() => {
    seq = 0
    URL.createObjectURL = vi.fn(() => `blob:http://x/${++seq}`)
    URL.revokeObjectURL = vi.fn()
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders text alone with no markers for no files', () => {
    const p = buildLocalPreview('你好', [])
    expect(p.content).toBe('你好')
    expect(p.objectURLs).toEqual([])
  })

  it('renders an uploaded image as an inline marker after the text', () => {
    const p = buildLocalPreview('看这张图', [new File(['x'], 'pic.png', { type: 'image/png' })])
    expect(p.content).toBe('看这张图\n![图片](blob:http://x/1)')
    expect(p.objectURLs).toEqual(['blob:http://x/1'])
  })

  it('renders a non-image upload as a file-card marker (no 附件 note)', () => {
    const p = buildLocalPreview('处理下', [
      new File(['x'], '黄山.docx', {
        type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      }),
    ])
    expect(p.content).toBe('处理下\n[file:黄山.docx](blob:http://x/1)')
    expect(p.content).not.toContain('（附件：')
  })

  it('attachment-only message contains only markers', () => {
    const p = buildLocalPreview('   ', [new File(['x'], 'a.png', { type: 'image/png' })])
    expect(p.content).toBe('![图片](blob:http://x/1)')
  })

  it('preserves upload order for mixed files', () => {
    const p = buildLocalPreview('都看看', [
      new File(['x'], 'doc.pdf', { type: 'application/pdf' }),
      new File(['x'], 'pic.jpg', { type: 'image/jpeg' }),
    ])
    expect(p.content.split('\n').slice(1)).toEqual([
      '[file:doc.pdf](blob:http://x/1)',
      '![图片](blob:http://x/2)',
    ])
    expect(p.objectURLs).toHaveLength(2)
  })
})

describe('extractBlobURLs', () => {
  it('returns distinct blob urls referenced by contents', () => {
    const urls = extractBlobURLs([
      'a\n![图片](blob:http://x/1)',
      'b\n[file:n.docx](blob:http://x/2)',
      'plain',
      'dup ![图片](blob:http://x/1)',
    ])
    expect(urls.sort()).toEqual(['blob:http://x/1', 'blob:http://x/2'])
  })

  it('ignores non-blob media urls', () => {
    expect(extractBlobURLs(['![图片](/v0/channels/media/c/a.png)'])).toEqual([])
  })
})
