import { describe, expect, it } from 'vitest'
import { splitChannelAttachments } from './channelImages'

describe('splitChannelAttachments', () => {
  it('extracts image urls and strips the image marker line', () => {
    const content =
      '看这张图（附件：image_abc.jpg）\n![图片](/v0/channels/media/weixin:acc:peer/obj.jpg)'
    const { text, images, files } = splitChannelAttachments(content)
    expect(images).toEqual([
      { name: '', url: '/v0/channels/media/weixin:acc:peer/obj.jpg', document: false },
    ])
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
      {
        name: '黄山三日行程.docx',
        url: '/v0/channels/media/weixin:a:p/obj.docx',
        document: false,
      },
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
    expect(images).toEqual([
      { name: '', url: 'blob:http://localhost/uuid-img', document: false },
    ])
    expect(files).toEqual([
      { name: '行程.docx', url: 'blob:http://localhost/uuid-doc', document: false },
    ])
    expect(text).toBe('刚发的图')
  })

  it('reads the original filename from the ?name= query for image deletion', () => {
    const content =
      '报告\n![图片](/v0/channels/media/c/a.pdf.png?name=%E6%8A%A5%E5%91%8A.png)'
    const { images } = splitChannelAttachments(content)
    expect(images).toEqual([
      {
        name: '报告.png',
        url: '/v0/channels/media/c/a.pdf.png?name=%E6%8A%A5%E5%91%8A.png',
        document: false,
      },
    ])
  })

  it('marks page images extracted from a document', () => {
    const content =
      '扫描件\n![文档图片](/v0/channels/media/c/p.jpg?name=scan.pdf%20%C2%B7%20page-1&from=doc)'
    const { images, text } = splitChannelAttachments(content)
    expect(images).toEqual([
      {
        name: 'scan.pdf · page-1',
        url: '/v0/channels/media/c/p.jpg?name=scan.pdf%20%C2%B7%20page-1&from=doc',
        document: true,
      },
    ])
    expect(text).toBe('扫描件')
  })
})
