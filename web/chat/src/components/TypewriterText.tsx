import { MarkdownText } from './MarkdownText'

export function TypewriterText({
  text,
  active,
}: {
  text: string
  active: boolean
}) {
  return (
    <div className="typewriter">
      <MarkdownText text={text} />
      {active && <span className="typewriter-caret" aria-hidden />}
    </div>
  )
}
