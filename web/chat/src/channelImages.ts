/**
 * Inbound channel images (e.g. WeChat photos) are appended to a persisted user
 * message as markdown image references pointing at the conversation-ACL
 * protected endpoint:
 *
 *   你好（附件：image_xxx.jpg）
 *   ![图片](/v0/channels/media/<conv>/<object>.jpg)
 *
 * The chat bubble shows the text (without the raw markdown line, which the
 * plain renderer would otherwise print literally) and renders each image via
 * <ChannelImage>. This splits those references out.
 */

const IMAGE_RE = /!\[([^\]]*)\]\((\/v0\/channels\/media\/[^)\s]+)\)/g

export interface SplitChannelImages {
  text: string
  images: string[]
}

export function splitChannelImages(content: string): SplitChannelImages {
  const images: string[] = []
  const text = content.replace(IMAGE_RE, (_m, _alt, url: string) => {
    images.push(url)
    return ''
  })
  return { text: text.replace(/\n{2,}$/g, '').trimEnd(), images }
}
