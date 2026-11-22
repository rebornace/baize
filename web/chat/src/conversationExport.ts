import type { ConversationExportFormat } from './api'

/** Portable filename for a conversation export. */
export function conversationExportFilename(
  conversationId: string,
  format: ConversationExportFormat = 'markdown',
): string {
  const ext = format === 'json' ? 'json' : format === 'html' ? 'html' : 'md'
  return `baize-${conversationId.replace(/[^a-zA-Z0-9._-]+/g, '-')}.${ext}`
}

export function conversationExportMime(format: ConversationExportFormat): string {
  switch (format) {
    case 'json':
      return 'application/json;charset=utf-8'
    case 'html':
      return 'text/html;charset=utf-8'
    default:
      return 'text/markdown;charset=utf-8'
  }
}

/** Trigger a browser download for text content. */
export function downloadTextFile(
  filename: string,
  text: string,
  mime = 'text/markdown;charset=utf-8',
): void {
  const blob = new Blob([text], { type: mime })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}
