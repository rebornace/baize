/** Display label for a conversation in the sidebar list. */
export function conversationListLabel(id: string, title: string): string {
  const base = title.trim() || '新对话'
  if (id.startsWith('weixin:')) {
    return `微信 · ${base}`
  }
  return base
}
