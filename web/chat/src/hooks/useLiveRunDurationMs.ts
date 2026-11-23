import { useEffect, useState } from 'react'
import type { Event } from '../api'
import { runDurationMsFromEvents } from '../runDuration'

/** Live wall duration: event span while idle; clock from first event while busy. */
export function useLiveRunDurationMs(
  events: ReadonlyArray<Pick<Event, 'timestamp'>>,
  busy: boolean,
): number | undefined {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!busy || events.length === 0) return
    setNow(Date.now())
    const id = window.setInterval(() => setNow(Date.now()), 500)
    return () => window.clearInterval(id)
  }, [busy, events.length])

  if (events.length === 0) return undefined
  return runDurationMsFromEvents(events, busy ? { untilMs: now } : undefined)
}
