import { inferMediaType, isImageAttachment } from './api'

export interface LocalPreview {
  /**
   * Optimistic bubble content: the typed text followed by media reference
   * lines that point at transient blob: object URLs, using the same marker
   * grammar as persisted messages (![图片](…)/[file:…](…)). Rendered locally
   * until the server version (with /v0/channels/media URLs) replaces it.
   */
  content: string
  /** Object URLs created for this preview; revoke them once replaced. */
  objectURLs: string[]
}

/**
 * Builds an optimistic user bubble for a just-sent message so an uploaded
 * image previews inline and a file shows as a download card immediately —
 * showing exactly what was sent, with no redundant "（附件：…）" text. Image
 * vs file is decided from the same MIME inference the upload uses.
 */
export function buildLocalPreview(text: string, files: readonly File[]): LocalPreview {
  const trimmed = text.trim()
  const objectURLs: string[] = []
  const markers: string[] = []

  for (const file of files) {
    const url = URL.createObjectURL(file)
    objectURLs.push(url)
    if (isImageAttachment(inferMediaType(file))) {
      markers.push(`![图片](${url})`)
    } else {
      markers.push(`[file:${file.name}](${url})`)
    }
  }

  let content = trimmed
  if (markers.length > 0) {
    content = trimmed ? `${trimmed}\n${markers.join('\n')}` : markers.join('\n')
  }
  return { content, objectURLs }
}

const BLOB_URL_RE = /blob:[^)\s]+/g

/** Returns the distinct transient blob: URLs referenced in message contents. */
export function extractBlobURLs(contents: readonly string[]): string[] {
  const found = new Set<string>()
  for (const c of contents) {
    for (const m of c.matchAll(BLOB_URL_RE)) found.add(m[0])
  }
  return [...found]
}
