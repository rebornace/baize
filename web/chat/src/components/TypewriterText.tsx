import { MarkdownText } from './MarkdownText'
import { useSmoothStreamText } from '../useSmoothStreamText'

/**
 * Live assistant text: server `text` is the target buffer; the visible string
 * is paced locally (adaptive drain). While `active`, always render plain text
 * so we never flip plain↔markdown mid-stream (that layout swap caused jitter).
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
