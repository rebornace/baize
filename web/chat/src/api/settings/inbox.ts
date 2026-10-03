import { authInit, parseJSON } from '../http'

export type InboxChannel = {
  id: string
  agent_id: string
  enabled: boolean
  skills?: string[]
  description?: string
  webhook_url?: string
  webhook_headers?: Record<string, string>
  secret_hint?: string
}

export async function getInboxChannels(): Promise<InboxChannel[]> {
  const res = await fetch('/v0/settings/inbox-channels', { headers: authInit() })
  const body = await parseJSON<{ channels: InboxChannel[] }>(res)
  return body.channels ?? []
}

export async function putInboxChannels(channels: InboxChannel[]): Promise<void> {
  const res = await fetch('/v0/settings/inbox-channels', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ channels }),
  })
  await parseJSON<{ channels: InboxChannel[] }>(res)
}

export async function rotateInboxSecret(id: string): Promise<{ secret: string }> {
  const res = await fetch(`/v0/settings/inbox-channels/${encodeURIComponent(id)}/rotate-secret`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<{ secret: string }>(res)
}

export async function testInboxChannel(
  id: string,
): Promise<{ delivery_id: string; run_id: string }> {
  const res = await fetch(`/v0/settings/inbox-channels/${encodeURIComponent(id)}/test`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<{ delivery_id: string; run_id: string }>(res)
}
