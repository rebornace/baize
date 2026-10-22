/** Reveal backlog over roughly this horizon (jitter buffer). */
export const DRAIN_HORIZON_MS = 380
/** Floor so a near-empty backlog still trickles instead of freezing. */
export const MIN_CPS = 48
/** Cap so a huge backlog (reconnect / burst) still looks like typing, not a dump. */
export const MAX_CPS = 900
/**
 * If the target is already longer than this when animation starts, snap —
 * treat as resume/history, not a fresh stream to replay from char 0.
 */
export const RESUME_SKIP_THRESHOLD = 64

/** Chars/second that drains `remaining` over the horizon, clamped. */
export function drainRate(remaining: number): number {
  if (remaining <= 0) return MIN_CPS
  return Math.min(MAX_CPS, Math.max(MIN_CPS, (remaining * 1000) / DRAIN_HORIZON_MS))
}

let segmenter: Intl.Segmenter | null | undefined

function getSegmenter(): Intl.Segmenter | null {
  if (segmenter !== undefined) return segmenter
  try {
    if (typeof Intl !== 'undefined' && 'Segmenter' in Intl) {
      segmenter = new Intl.Segmenter(undefined, { granularity: 'grapheme' })
      return segmenter
    }
  } catch {
    /* ignore */
  }
  segmenter = null
  return null
}

/** Next index after consuming up to `budget` grapheme clusters from `start`. */
export function advanceByGraphemes(text: string, start: number, budget: number): number {
  if (budget < 1 || start >= text.length) return start
  const seg = getSegmenter()
  if (seg) {
    let i = start
    let n = 0
    for (const { index, segment } of seg.segment(text.slice(start))) {
      if (n >= budget) break
      i = start + index + segment.length
      n++
    }
    return i
  }
  // Code-point fallback (keeps surrogate pairs together).
  let i = start
  let n = 0
  while (i < text.length && n < budget) {
    const cp = text.codePointAt(i)
    if (cp === undefined) break
    i += cp > 0xffff ? 2 : 1
    n++
  }
  return i
}
