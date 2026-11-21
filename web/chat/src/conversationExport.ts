/** Portable Markdown filename for a conversation export. */
export function conversationExportFilename(conversationId: string): string {
  return `baize-${conversationId.replace(/[^a-zA-Z0-9._-]+/g, '-')}.md`
}

/** Trigger a browser download for text content. */
export function downloadTextFile(filename: string, text: string): void {
  const blob = new Blob([text], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}
