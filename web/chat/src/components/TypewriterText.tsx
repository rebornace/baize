import { MarkdownText } from './MarkdownText'
import { useSmoothStreamText } from '../useSmoothStreamText'

/**
 * Live assistant text: shows the server `text` snapshot immediately (no local
 * drain). While `active`, render plain text so we never flip plain↔markdown
 * mid-stream (that layout swap caused jitter).
 */
export function TypewriterText({
  text,
  active,
}: {
  text: string
  active: boolean
}) {
  const displayed = useSmoothStreamText(text, active)

  return (
    <div className={`typewriter${active ? ' is-streaming' : ''}`}>
      {active ? (
        <div className="md-body md-plain typewriter-plain">
          {displayed}
          <span className="typewriter-caret" aria-hidden />
        </div>
      ) : (
        <MarkdownText text={displayed} />
      )}
    </div>
  )
}
