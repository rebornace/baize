import { describe, expect, it } from 'vitest'
import { splitChannelAttachments } from './channelImages'

describe('splitChannelAttachments', () => {
  it('extracts image urls and strips the image marker line', () => {
    const content =
      '看这张图（附件：image_abc.jpg）\n![图片](/v0/channels/media/weixin:acc:peer/obj.jpg)'
    const { text, images, files } = splitChannelAttachments(content)
    expect(images).toEqual(['/v0/channels/media/weixin:acc:peer/obj.jpg'])
    expect(files).toHaveLength(0)
    expect(text).toContain('看这张图（附件：image_abc.jpg）')
    expect(text).not.toContain('![图片]')
    expect(text).not.toContain('/v0/channels/media/')
  })

  it('extracts downloadable file links with names', () => {
    const content =
      '行程（附件：黄山三日行程.docx）\n[file:黄山三日行程.docx](/v0/channels/media/weixin:a:p/obj.docx)'
    const { text, images, files } = splitChannelAttachments(content)
    expect(images).toHaveLength(0)
    expect(files).toEqual([
      { name: '黄山三日行程.docx', url: '/v0/channels/media/weixin:a:p/obj.docx' },
    ])
    expect(text).toContain('行程（附件：黄山三日行程.docx）')
    expect(text).not.toContain('[file:')
  })

  it('handles multiple images', () => {
    const content =
      '两张\n![图片](/v0/channels/media/c/a.png)\n![图片](/v0/channels/media/c/b.png)'
    const { images } = splitChannelAttachments(content)
    expect(images).toHaveLength(2)
  })

  it('handles a mix of images and files', () => {
    const content =
      '图和文件\n![图片](/v0/channels/media/c/a.png)\n[file:doc.pdf](/v0/channels/media/c/b.pdf)'
    const { images, files } = splitChannelAttachments(content)
    expect(images).toHaveLength(1)
    expect(files).toHaveLength(1)
  })

  it('leaves normal text untouched', () => {
    const { text, images, files } = splitChannelAttachments('普通消息，没有附件')
    expect(images).toHaveLength(0)
    expect(files).toHaveLength(0)
    expect(text).toBe('普通消息，没有附件')
  })

  it('does not treat unrelated markdown links as attachments', () => {
    const { images, files } = splitChannelAttachments('[link](https://example.com)')
    expect(images).toHaveLength(0)
    expect(files).toHaveLength(0)
  })

  it('parses optimistic blob: preview markers (just-sent web uploads)', () => {
    const content =
      '刚发的图\n' +
      '![图片](blob:http://localhost/uuid-img)\n' +
      '[file:行程.docx](blob:http://localhost/uuid-doc)'
    const { text, images, files } = splitChannelAttachments(content)
    expect(images).toEqual(['blob:http://localhost/uuid-img'])
    expect(files).toEqual([{ name: '行程.docx', url: 'blob:http://localhost/uuid-doc' }])
    expect(text).toBe('刚发的图')
  })
})
