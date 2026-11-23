import { useCallback, useEffect, useRef } from 'react'

/** Keeps a scroll container pinned to the bottom unless the user scrolls up. */
export function useStickToBottom(deps: readonly unknown[]) {
  const scrollerRef = useRef<HTMLDivElement | null>(null)
  const bottomRef = useRef<HTMLDivElement | null>(null)
  const stickRef = useRef(true)
  const rafPinRef = useRef<number | null>(null)

  const pinNow = useCallback(() => {
    if (!stickRef.current) return
    const el = scrollerRef.current
    if (el) {
      el.scrollTop = el.scrollHeight
      return
    }
    bottomRef.current?.scrollIntoView({ behavior: 'auto', block: 'end' })
  }, [])

  /** Coalesce rapid height changes (typewriter) into one scroll per frame. */
  const schedulePin = useCallback(() => {
    if (!stickRef.current) return
    if (rafPinRef.current !== null) return
    rafPinRef.current = window.requestAnimationFrame(() => {
      rafPinRef.current = null
      pinNow()
    })
  }, [pinNow])

  const onScroll = useCallback(() => {
    const el = scrollerRef.current
    if (!el) return
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight
    stickRef.current = distance < 96
  }, [])

  const scrollToBottom = useCallback((behavior: ScrollBehavior = 'auto') => {
    stickRef.current = true
    const el = scrollerRef.current
    if (el) {
      if (behavior === 'smooth') {
        el.scrollTo({ top: el.scrollHeight, behavior })
      } else {
        el.scrollTop = el.scrollHeight
      }
      return
    }
    bottomRef.current?.scrollIntoView({ behavior, block: 'end' })
  }, [])

  useEffect(() => {
    pinNow()
    // Intentional: follow caller-provided content deps (messages / live events).
  }, deps)

  // Typewriter grows height without changing liveEvents; keep stick smooth.
  useEffect(() => {
    const el = scrollerRef.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const inner = el.firstElementChild
    if (!inner) return
    const ro = new ResizeObserver(() => {
      schedulePin()
    })
    ro.observe(inner)
    return () => {
      ro.disconnect()
      if (rafPinRef.current !== null) {
        window.cancelAnimationFrame(rafPinRef.current)
        rafPinRef.current = null
      }
    }
  }, [schedulePin])

  return { scrollerRef, bottomRef, onScroll, scrollToBottom, stickRef }
}
