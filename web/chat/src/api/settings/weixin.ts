import { authInit, parseJSON } from '../http'


export interface WeixinChannelSettings {
  agent_id: string
  allowlist: string[]
  assignee: string
  enabled: boolean
  /** Runtime reconciled state, only present on the PUT response. */
  running?: boolean
  /** Why the channel is not running: "login_required" | "login_expired" | "start_failed". */
  reason?: string
}

export interface WeixinLoginStart {
  ticket: string
  qr_url: string
}

export interface WeixinLoginStatus {
  status: string
}

export async function startWeixinLogin(): Promise<WeixinLoginStart> {
  const res = await fetch('/v0/settings/channels/weixin/login/start', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<WeixinLoginStart>(res)
}

export async function getWeixinLoginStatus(ticket: string): Promise<WeixinLoginStatus> {
  const qs = new URLSearchParams({ ticket })
  const res = await fetch(`/v0/settings/channels/weixin/login/status?${qs}`, {
    headers: authInit(),
  })
  return parseJSON<WeixinLoginStatus>(res)
}

export async function logoutWeixin(): Promise<{ status: string }> {
  const res = await fetch('/v0/settings/channels/weixin/logout', {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<{ status: string }>(res)
}

export async function getWeixinSettings(): Promise<WeixinChannelSettings> {
  const res = await fetch('/v0/settings/channels/weixin', { headers: authInit() })
  return parseJSON<WeixinChannelSettings>(res)
}

export async function putWeixinSettings(
  body: WeixinChannelSettings,
): Promise<WeixinChannelSettings> {
  const res = await fetch('/v0/settings/channels/weixin', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<WeixinChannelSettings>(res)
}

/**
 * Adapter process lifecycle control. These act on the OS process (start / stop
 * / restart the weixin-adapter), distinct from the enabled toggle which only
 * starts/stops polling. Each returns the reconciled settings + running state.
 */
async function postWeixinProcess(action: 'start' | 'stop' | 'restart'): Promise<WeixinChannelSettings> {
  const res = await fetch(`/v0/settings/channels/weixin/process/${action}`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<WeixinChannelSettings>(res)
}

export const startWeixinProcess = () => postWeixinProcess('start')
export const stopWeixinProcess = () => postWeixinProcess('stop')
export const restartWeixinProcess = () => postWeixinProcess('restart')

export type ChannelOutboundDelivery = {
  id: string
  status: string
  kind: string
  peer_id?: string
  conversation_id?: string
  run_id?: string
  attempt: number
  max_attempts: number
  last_error?: string
  updated_at?: string
}

export async function getChannelOutboundDeliveries(
  channel: string,
  params?: { status?: string; limit?: number },
): Promise<ChannelOutboundDelivery[]> {
  const qs = new URLSearchParams()
  if (params?.status) qs.set('status', params.status)
  if (params?.limit != null) qs.set('limit', String(params.limit))
  const suffix = qs.toString() ? `?${qs.toString()}` : ''
  const res = await fetch(
    `/v0/settings/channels/${encodeURIComponent(channel)}/outbound-deliveries${suffix}`,
    { headers: authInit() },
  )
  const body = await parseJSON<{ deliveries: ChannelOutboundDelivery[] }>(res)
  return body.deliveries ?? []
}

export async function retryChannelOutboundDelivery(
  channel: string,
  id: string,
): Promise<{ status: string }> {
  const res = await fetch(
    `/v0/settings/channels/${encodeURIComponent(channel)}/outbound-deliveries/${encodeURIComponent(id)}/retry`,
    {
      method: 'POST',
      headers: authInit(),
    },
  )
  return parseJSON<{ status: string }>(res)
}
