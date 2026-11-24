import { authInit, parseJSON } from './http'

export interface ConversationAttachment {
  object: string
  filename: string
  media_type: string
  size_bytes: number
  created_at: string
  url: string
}

export async function listConversationAttachments(
  conversationId: string,
  limit = 100,
): Promise<{ items: ConversationAttachment[] }> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/attachments?limit=${limit}`,
    { headers: authInit() },
  )
  return parseJSON(res)
}

export async function deleteConversationAttachments(
  conversationId: string,
  objects: string[],
): Promise<{ deleted: number; failed: number }> {
  const res = await fetch(
    `/v0/conversations/${encodeURIComponent(conversationId)}/attachments/delete`,
    {
      method: 'POST',
      headers: authInit({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ objects }),
    },
  )
  return parseJSON(res)
}
