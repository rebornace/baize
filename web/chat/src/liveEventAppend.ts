import type { Event } from './api'

const COALESCE_TYPES = new Set(['llm.content.delta', 'llm.thinking.delta'])

function eventTurn(ev: Event): number {
  const t = ev.data?.turn
  return typeof t === 'number' && Number.isFinite(t) ? t : 0
}

/** True when this stream event only replaces the previous cumulative text for the same turn. */
export function isCoalescableDelta(ev: Event): boolean {
  return COALESCE_TYPES.has(ev.type)
}

/**
 * Append a live run event, replacing the previous content/thinking delta for the
 * same turn instead of stacking one event per token. Keeps foldEvents / React
 * work from growing with stream length (especially after many tool steps).
 */
export function appendLiveEvent(events: readonly Event[], ev: Event): Event[] {
  if (isCoalescableDelta(ev)) {
    const last = events[events.length - 1]
    if (last && last.type === ev.type && eventTurn(last) === eventTurn(ev)) {
      const next = events.slice(0, -1)
      next.push(ev)
      return next
    }
  }
  return events.concat(ev)
}

/** Collapse consecutive same-turn content/thinking deltas in a full event list (poll / restore). */
export function coalesceLiveEvents(events: readonly Event[]): Event[] {
  let out: Event[] = []
  for (const ev of events) {
    out = appendLiveEvent(out, ev)
  }
  return out
}
