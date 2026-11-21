import { authInit, parseJSON } from './http'

export interface ChannelInboundJob {
  id: string
  channel: string
  status: string
  attempt: number
  max_attempts: number
  last_error?: string
  next_attempt_at?: string
  created_at?: string
  updated_at?: string
}

export async function listChannelInboundJobs(): Promise<ChannelInboundJob[]> {
  const res = await fetch('/v0/settings/channels/inbound-jobs', { headers: authInit() })
  const body = await parseJSON<{ jobs?: ChannelInboundJob[] }>(res)
  return body.jobs ?? []
}

export async function retryChannelInboundJob(id: string): Promise<void> {
  const res = await fetch(
    `/v0/settings/channels/inbound-jobs/${encodeURIComponent(id)}/retry`,
    { method: 'POST', headers: authInit() },
  )
  await parseJSON<{ status: string }>(res)
}
