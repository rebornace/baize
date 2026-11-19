import type { Event } from './api'

/** Number of goal.* events in a live-event list; used to trigger a refresh. */
export function goalEventCount(events: ReadonlyArray<Event> | undefined): number {
  if (!events || events.length === 0) return 0
  let n = 0
  for (const ev of events) {
    if (typeof ev?.type === 'string' && ev.type.startsWith('goal.')) n++
  }
  return n
}
