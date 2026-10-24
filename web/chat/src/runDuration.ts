import type { Event } from './api'

/** Wall-clock span of a run from earliest to latest event timestamp (ms). */
export function runDurationMsFromEvents(
  events: ReadonlyArray<Pick<Event, 'timestamp'>>,
  opts?: { untilMs?: number },
): number | undefined {
  let min = Number.POSITIVE_INFINITY
  let max = Number.NEGATIVE_INFINITY
  for (const ev of events) {
    const t = Date.parse(ev.timestamp)
    if (!Number.isFinite(t)) continue
    if (t < min) min = t
    if (t > max) max = t
  }
  if (!Number.isFinite(min)) return undefined
  const end = opts?.untilMs ?? max
  if (!Number.isFinite(end)) return undefined
  return Math.max(0, Math.round(end - min))
}

/** Human wall duration for chat meta (no leading label). */
export function formatWallDuration(ms: number, locale: string): string {
  const secTotal = Math.max(0, ms) / 1000
  const zh = locale.startsWith('zh')
  if (secTotal < 60) {
    const s = secTotal < 10 ? secTotal.toFixed(1) : String(Math.round(secTotal))
    return zh ? `${s} 秒` : `${s}s`
  }
  const totalSec = Math.round(secTotal)
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  if (h > 0) {
    return zh
      ? `${h} 小时 ${String(m).padStart(2, '0')} 分`
      : `${h}h ${String(m).padStart(2, '0')}m`
  }
  return zh
    ? `${m} 分 ${String(s).padStart(2, '0')} 秒`
    : `${m}m ${String(s).padStart(2, '0')}s`
}
