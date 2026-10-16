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

const datetimeLocalPattern = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/

function timeZoneParts(date: Date, timeZone: string) {
  const dtf = new Intl.DateTimeFormat('en-US', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  })
  const parts = dtf.formatToParts(date)
  const pick = (type: string) => parts.find((p) => p.type === type)?.value ?? '0'
  return {
    year: Number(pick('year')),
    month: Number(pick('month')),
    day: Number(pick('day')),
    hour: Number(pick('hour')),
    minute: Number(pick('minute')),
    second: Number(pick('second')),
  }
}

/** datetime-local (no offset) interpreted in IANA timeZone → RFC3339 UTC (Z). */
export function onceFireAtRFC3339(datetimeLocal: string, timeZone: string): string {
  const trimmed = datetimeLocal.trim()
  const m = datetimeLocalPattern.exec(trimmed)
  if (!m) {
    return trimmed
  }
  const want = {
    year: Number(m[1]),
    month: Number(m[2]),
    day: Number(m[3]),
    hour: Number(m[4]),
    minute: Number(m[5]),
  }
  const tz = timeZone.trim() || 'UTC'
  let utc = Date.UTC(want.year, want.month - 1, want.day, want.hour, want.minute, 0)
  for (let i = 0; i < 8; i++) {
    const got = timeZoneParts(new Date(utc), tz)
    const delta =
      Date.UTC(want.year, want.month - 1, want.day, want.hour, want.minute, 0) -
      Date.UTC(got.year, got.month - 1, got.day, got.hour, got.minute, got.second)
    if (delta === 0) {
      break
    }
    utc += delta
  }
  return new Date(utc).toISOString()
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
    case 'once': {
      const fireAtRfc = onceFireAtRFC3339(fire_at ?? '', timezone ?? 'UTC')
      return JSON.stringify({
        kind,
        conversation_id,
        fire_at: fireAtRfc,
        timezone,
        instruction,
      })
    }
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
