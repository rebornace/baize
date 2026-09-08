import { describe, expect, it } from 'vitest'
import { splitChannelImages } from './channelImages'

describe('splitChannelImages', () => {
  it('extracts channel media image urls and strips the markdown line', () => {
    const content =
      '看这张图（附件：image_abc.jpg）\n![图片](/v0/channels/media/weixin:acc:peer/obj.jpg)'
    const { text, images } = splitChannelImages(content)
    expect(images).toEqual(['/v0/channels/media/weixin:acc:peer/obj.jpg'])
    expect(text).toContain('看这张图（附件：image_abc.jpg）')
    expect(text).not.toContain('![图片]')
    expect(text).not.toContain('/v0/channels/media/')
  })

  it('handles multiple images', () => {
    const content =
      '两张\n![图片](/v0/channels/media/c/a.png)\n![图片](/v0/channels/media/c/b.png)'
    const { images } = splitChannelImages(content)
    expect(images).toHaveLength(2)
  })

  it('leaves normal text untouched', () => {
    const { text, images } = splitChannelImages('普通消息，没有图片')
    expect(images).toHaveLength(0)
    expect(text).toBe('普通消息，没有图片')
  })

  it('does not treat unrelated markdown links as images', () => {
    const { images } = splitChannelImages('[link](https://example.com)')
    expect(images).toHaveLength(0)
  })
})
