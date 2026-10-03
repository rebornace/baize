import { useCallback, useEffect, useRef } from 'react'

/** Keeps a scroll container pinned to the bottom unless the user scrolls up. */
export function useStickToBottom(deps: readonly unknown[]) {
  const scrollerRef = useRef<HTMLDivElement | null>(null)
  const bottomRef = useRef<HTMLDivElement | null>(null)
  const stickRef = useRef(true)

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
    if (!stickRef.current) return
    const el = scrollerRef.current
    if (el) {
      el.scrollTop = el.scrollHeight
    } else {
      bottomRef.current?.scrollIntoView({ behavior: 'auto', block: 'end' })
    }
    // Intentional: follow caller-provided content deps (messages / live events).
  }, deps)

  return { scrollerRef, bottomRef, onScroll, scrollToBottom, stickRef }
}
