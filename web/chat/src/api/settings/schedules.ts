import { authInit, parseJSON } from '../http'

export interface ScheduleEntry {
  id: string
  owner_id: string
  conversation_id: string
  instruction: string
  kind: string
  timezone: string
  fire_at?: string
  time_of_day?: string
  weekday?: number
  status: string
  next_run_at: string
  running_run_id?: string
  last_error?: string
  revision: number
}

export type CreateScheduleBody = {
  conversation_id: string
  kind: string
  instruction: string
  timezone?: string
  fire_at?: string
  time_of_day?: string
  weekday?: number
}

export async function listSchedules(): Promise<ScheduleEntry[]> {
  const res = await fetch('/v0/settings/schedules', { headers: authInit() })
  const body = await parseJSON<{ items: ScheduleEntry[] }>(res)
  return body.items ?? []
}

function schedulePostPayload(body: CreateScheduleBody): string {
  const kind = body.kind
  const conversation_id = body.conversation_id
  const instruction = body.instruction
  const timezone = body.timezone
  const time_of_day = body.time_of_day
  const fire_at = body.fire_at
  const weekday = body.weekday
  switch (kind) {
    case 'daily':
      return JSON.stringify({
        kind,
        conversation_id,
        time_of_day,
        timezone,
        instruction,
      })
    case 'weekly':
      return JSON.stringify({
        kind,
        conversation_id,
        time_of_day,
        timezone,
        weekday,
        instruction,
      })
    case 'once':
      return JSON.stringify({
        kind,
        conversation_id,
        fire_at,
        timezone,
        instruction,
      })
    default:
      return JSON.stringify(body)
  }
}

export async function createSchedule(body: CreateScheduleBody): Promise<ScheduleEntry> {
  const res = await fetch('/v0/settings/schedules', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: schedulePostPayload(body),
  })
  return parseJSON<ScheduleEntry>(res)
}

export async function pauseSchedule(id: string): Promise<ScheduleEntry> {
  const res = await fetch(`/v0/settings/schedules/${encodeURIComponent(id)}/pause`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<ScheduleEntry>(res)
}

export async function resumeSchedule(id: string): Promise<ScheduleEntry> {
  const res = await fetch(`/v0/settings/schedules/${encodeURIComponent(id)}/resume`, {
    method: 'POST',
    headers: authInit(),
  })
  return parseJSON<ScheduleEntry>(res)
}

export async function deleteSchedule(id: string): Promise<void> {
  const res = await fetch(`/v0/settings/schedules/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authInit(),
  })
  await parseJSON<{ status: string }>(res)
}
