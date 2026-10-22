import { useEffect, useRef, useState } from 'react'
import {
  RESUME_SKIP_THRESHOLD,
  advanceByGraphemes,
  drainRate,
} from './smoothStream'

/**
 * Smooth streaming reveal: `target` is the latest server snapshot; the UI
 * drains a local buffer at an adaptive rate so bursty SSE does not stutter
 * and a slow model still shows steady motion while backlog remains.
 *
 * Same playout-buffer idea as Onyx / Sim / smooth-streaming hosts: reveal is
 * paced locally, not "paint whatever the last network packet was".
 */
export function useSmoothStreamText(target: string, active: boolean): string {
  const [displayed, setDisplayed] = useState(() => {
    if (!active) return target
    // Only skip replay for large already-complete snapshots (reconnect mid-run).
    // Fresh streams often start empty then grow; never snap the first paint to full.
    if (target.length >= RESUME_SKIP_THRESHOLD) return target
    return ''
  })

  const displayedRef = useRef(displayed)
  const targetRef = useRef(target)
  const carryRef = useRef(0)
  const lastTsRef = useRef<number | null>(null)
  const lastPaintRef = useRef(0)

  targetRef.current = target

  // Inactive (history): always show the full target.
  useEffect(() => {
    if (active) return
    displayedRef.current = target
    setDisplayed(target)
    carryRef.current = 0
    lastTsRef.current = null
  }, [active, target])

  // Active: if the stream replaced text (not a growing prefix), resync.
  useEffect(() => {
    if (!active) return
    const d = displayedRef.current
    if (target.startsWith(d)) return
    displayedRef.current = target
    setDisplayed(target)
    carryRef.current = 0
  }, [active, target])

  useEffect(() => {
    if (!active) return

    let raf = 0
    const tick = (now: number) => {
      const prevTs = lastTsRef.current
      lastTsRef.current = now
      const dt = prevTs == null ? 16 : Math.min(50, Math.max(0, now - prevTs))

      const t = targetRef.current
      let d = displayedRef.current

      if (!t.startsWith(d)) {
        d = t
        displayedRef.current = d
        setDisplayed(d)
        carryRef.current = 0
        raf = requestAnimationFrame(tick)
        return
      }

      const remUnits = countGraphemes(t.slice(d.length))
      if (remUnits <= 0) {
        carryRef.current = 0
        // Flush any throttled tail so the last graphemes are not stuck off-screen.
        setDisplayed(d)
        raf = requestAnimationFrame(tick)
        return
      }

      const cps = drainRate(remUnits)
      carryRef.current += (cps * dt) / 1000
      const budget = Math.floor(carryRef.current)
      if (budget < 1) {
        raf = requestAnimationFrame(tick)
        return
      }
      carryRef.current -= budget

      const next = advanceByGraphemes(t, d.length, budget)
      if (next !== d.length) {
        d = t.slice(0, next)
        displayedRef.current = d
        // ~30fps paints: fewer layout passes than every rAF when catching up fast.
        const caughtUp = d.length >= t.length
        if (caughtUp || now - lastPaintRef.current >= 32) {
          lastPaintRef.current = now
          setDisplayed(d)
        }
      }
      raf = requestAnimationFrame(tick)
    }

    raf = requestAnimationFrame(tick)
    return () => {
      cancelAnimationFrame(raf)
      lastTsRef.current = null
    }
  }, [active])

  return displayed
}

function countGraphemes(s: string): number {
  if (!s) return 0
  try {
    if (typeof Intl !== 'undefined' && 'Segmenter' in Intl) {
      const seg = new Intl.Segmenter(undefined, { granularity: 'grapheme' })
      let n = 0
      for (const _ of seg.segment(s)) n++
      return n
    }
  } catch {
    /* fall through */
  }
  return [...s].length
}
