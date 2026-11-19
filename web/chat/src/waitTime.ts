export type WaitedUnits = {
  waitedMinutes: (n: number) => string
  waitedHours: (n: number) => string
  waitedDays: (n: number) => string
}

/** Formats how long ago `since` was, using the caller's locale strings. */
export function waitedLabel(
  since: string | undefined,
  now: number,
  units: WaitedUnits,
): string | null {
  if (!since) return null
  const start = Date.parse(since)
  if (!Number.isFinite(start)) return null
  const ms = Math.max(0, now - start)
  const minutes = Math.floor(ms / 60_000)
  if (minutes < 60) return units.waitedMinutes(Math.max(1, minutes))
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return units.waitedHours(hours)
  return units.waitedDays(Math.floor(hours / 24))
}
