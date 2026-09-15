import { authInit, parseJSON } from '../http'

export interface EventsWebhookConfig {
  url: string
  headers: Record<string, string>
}

export async function getEventsWebhook(): Promise<EventsWebhookConfig> {
  const res = await fetch('/v0/settings/events-webhook', { headers: authInit() })
  return parseJSON<EventsWebhookConfig>(res)
}

export async function putEventsWebhook(body: EventsWebhookConfig): Promise<EventsWebhookConfig> {
  const res = await fetch('/v0/settings/events-webhook', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<EventsWebhookConfig>(res)
}

export async function testEventsWebhook(): Promise<{ status: string }> {
  const res = await fetch('/v0/settings/events-webhook/test', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<{ status: string }>(res)
}

export interface EventsWebhookDelivery {
  id: string
  run_id: string
  kind: string
  event_index: number
  status: string
  attempt: number
  max_attempts: number
  last_error?: string
  next_retry_at: string
  created_at: string
  updated_at: string
}

export async function getEventsWebhookDeliveries(params?: {
  status?: string
  limit?: number
}): Promise<EventsWebhookDelivery[]> {
  const qs = new URLSearchParams()
  if (params?.status) qs.set('status', params.status)
  if (params?.limit != null) qs.set('limit', String(params.limit))
  const suffix = qs.toString() ? `?${qs.toString()}` : ''
  const res = await fetch(`/v0/settings/events-webhook/deliveries${suffix}`, {
    headers: authInit(),
  })
  const body = await parseJSON<{ deliveries: EventsWebhookDelivery[] }>(res)
  return body.deliveries ?? []
}

export async function retryEventsWebhookDelivery(id: string): Promise<{ status: string }> {
  const res = await fetch(`/v0/settings/events-webhook/deliveries/${encodeURIComponent(id)}/retry`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<{ status: string }>(res)
}
