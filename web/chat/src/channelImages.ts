/**
 * Inbound channel attachments (e.g. WeChat photos / files) are appended to a
 * persisted user message as references to the conversation-ACL protected
 * endpoint:
 *
 *   你好（附件：image_xxx.jpg）
 *   ![图片](/v0/channels/media/<conv>/<object>.jpg)          // inline image
 *   [file:黄山三日行程.docx](/v0/channels/media/<conv>/<object>.docx)  // download
 *
 * The chat bubble shows the text (without those raw marker lines, which the
 * plain renderer would otherwise print literally), renders each image inline,
 * and each non-image file as a download link. This splits the references out.
 */

// Persisted references point at the conversation-ACL media endpoint; the
// optimistic bubble for a just-sent upload uses a transient blob: object URL
// (local preview until the server version replaces it), so both are accepted.
const MEDIA_URL = String.raw`(?:/v0/channels/media/[^)\s]+|blob:[^)\s]+)`
const IMAGE_RE = new RegExp(String.raw`!\[([^\]]*)\]\((${MEDIA_URL})\)`, 'g')
const FILE_RE = new RegExp(String.raw`\[file:([^\]]+)\]\((${MEDIA_URL})\)`, 'g')

export interface ChannelAttachment {
  name: string
  url: string
}

export interface SplitChannelAttachments {
  text: string
  images: string[]
  files: ChannelAttachment[]
}

export function splitChannelAttachments(content: string): SplitChannelAttachments {
  const images: string[] = []
  const files: ChannelAttachment[] = []

  const text = content
    .replace(IMAGE_RE, (_m, _alt, url: string) => {
      images.push(url)
      return ''
    })
    .replace(FILE_RE, (_m, name: string, url: string) => {
      files.push({ name, url })
      return ''
    })

  return {
    text: text.replace(/\n{2,}$/g, '').trimEnd(),
    images,
    files,
  }
}
