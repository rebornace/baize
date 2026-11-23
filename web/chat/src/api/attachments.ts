import type { Attachment } from './types'

// EXTENSION_MIME maps supported attachment extensions to the canonical MIME
// the backend's attach.Process accepts. We infer from the extension (rather
// than trusting File.type) because Windows often gives .md an empty type and
// .csv "application/vnd.ms-excel", which would otherwise be rejected as
// unsupported_attachment before the bytes are even inspected.
const EXTENSION_MIME: Record<string, string> = {
  '.txt': 'text/plain',
  '.md': 'text/markdown',
  '.csv': 'text/csv',
  '.docx': 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  '.xlsx': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  '.pdf': 'application/pdf',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.webp': 'image/webp',
  '.gif': 'image/gif',
}

/**
 * inferMediaType returns the canonical backend MIME for a file by extension,
 * falling back to the browser-provided type (or application/octet-stream) for
 * unknown extensions. This normalizes platform quirks (e.g. .csv reported as
 * application/vnd.ms-excel on Windows) so the backend doesn't reject a
 * supported file as unsupported_attachment.
 */
export function inferMediaType(file: File): string {
  const name = file.name.toLowerCase()
  const dot = name.lastIndexOf('.')
  if (dot >= 0) {
    const ext = name.slice(dot)
    const canonical = EXTENSION_MIME[ext]
    if (canonical) return canonical
  }
  return file.type || 'application/octet-stream'
}

/** Read a File into an Attachment (base64-encoded content). */
export function fileToAttachment(file: File): Promise<Attachment> {
  const media_type = inferMediaType(file)
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error ?? new Error('read failed'))
    reader.onload = () => {
      const result = reader.result
      if (typeof result !== 'string') {
        reject(new Error('unsupported file read result'))
        return
      }
      const comma = result.indexOf(',')
      const content_base64 = comma < 0 ? result : result.slice(comma + 1)
      resolve({
        filename: file.name,
        media_type,
        content_base64,
      })
    }
    reader.readAsDataURL(file)
  })
}

const IMAGE_MIMES = new Set([
  'image/png',
  'image/jpeg',
  'image/jpg',
  'image/webp',
  'image/gif',
])

/** True for image MIME types the backend accepts as vision attachments. */
export function isImageAttachment(mediaType: string): boolean {
  return IMAGE_MIMES.has(mediaType.toLowerCase())
}
