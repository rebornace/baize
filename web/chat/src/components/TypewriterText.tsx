import { useEffect, useState } from 'react'
import { MarkdownText } from './MarkdownText'

export function TypewriterText({
  text,
  active,
}: {
  text: string
  active: boolean
}) {
  const [shown, setShown] = useState(() => (active ? '' : text))

  useEffect(() => {
    if (!active) {
      setShown(text)
      return
    }
    if (shown === text) return
    if (!text.startsWith(shown)) {
      // Content was replaced (e.g. regenerate) — restart reveal.
      setShown('')
      return
    }
    const lag = text.length - shown.length
    const step = lag > 120 ? Math.ceil(lag / 5) : lag > 40 ? 3 : 1
    const delay = lag > 80 ? 12 : 16
    const timer = window.setTimeout(() => {
      setShown(text.slice(0, shown.length + step))
    }, delay)
    return () => window.clearTimeout(timer)
  }, [text, shown, active])

  return (
    <div className="typewriter">
      <MarkdownText text={shown} />
      {active && shown.length < text.length && <span className="typewriter-caret" aria-hidden />}
    </div>
  )
}
