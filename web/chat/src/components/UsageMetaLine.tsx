import type { UsageMeta } from '../foldEvents'
import { CHAT } from '../strings'

export type UsageMetaLineProps = {
  usage: UsageMeta
}

// UsageMetaLine renders real per-run token usage and/or the estimated tokens
// saved when the decision layer skipped memory extraction. Nothing renders
// when there is nothing to report.
export function UsageMetaLine({ usage }: UsageMetaLineProps) {
  const hasUsage = usage.totalTokens > 0
  const hasSaved = usage.savedTokens > 0
  if (!hasUsage && !hasSaved) return null

  let text: string
  if (hasUsage && hasSaved) {
    text = CHAT.usageBothLine(
      usage.promptTokens,
      usage.completionTokens,
      usage.totalTokens,
      usage.savedTokens,
    )
  } else if (hasUsage) {
    text = CHAT.usageLine(
      usage.promptTokens,
      usage.completionTokens,
      usage.totalTokens,
    )
  } else {
    text = CHAT.usageSavedLine(usage.savedTokens)
  }

  return (
    <p className="msg-usage" data-testid="usage-line">
      {text}
    </p>
  )
}
