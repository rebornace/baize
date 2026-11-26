import { authInit, parseJSON } from '../http'

export interface AlertChannel {
  id: string
  name: string
  type: 'feishu' | 'dingtalk' | 'wecom' | 'generic' | 'email'
  url: string
  to?: string[]
  enabled: boolean
}

export async function getAlertChannels(): Promise<{ items: AlertChannel[]; max: number }> {
  const res = await fetch('/v0/settings/alert-channels', { headers: authInit() })
  return parseJSON(res)
}

export async function putAlertChannels(
  items: AlertChannel[],
): Promise<{ items: AlertChannel[]; max: number }> {
  const res = await fetch('/v0/settings/alert-channels', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ items }),
  })
  return parseJSON(res)
}

export async function testAlertChannel(
  channel: AlertChannel,
): Promise<{ status: string }> {
  const res = await fetch('/v0/settings/alert-channels/test', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(channel),
  })
  return parseJSON(res)
}

export interface SMTPSettings {
  host: string
  port: number
  username: string
  from: string
  security: 'starttls' | 'tls' | 'none'
  has_password: boolean
}

export async function getSMTPSettings(): Promise<SMTPSettings> {
  const res = await fetch('/v0/settings/smtp', { headers: authInit() })
  return parseJSON(res)
}

export async function putSMTPSettings(body: {
  host: string
  port: number
  username: string
  password?: string
  clear_password?: boolean
  from: string
  security: SMTPSettings['security']
}): Promise<SMTPSettings> {
  const res = await fetch('/v0/settings/smtp', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON(res)
}
