import type { UsageMeta } from '../foldEvents'
import { CHAT } from '../strings'

export type UsageMetaLineProps = {
  usage: UsageMeta
}

function usageSummaryLabel(usage: UsageMeta, hasUsage: boolean, hasSaved: boolean): string {
  if (hasUsage && hasSaved) {
    return CHAT.usageSummaryBoth(usage.totalTokens, usage.savedTokens)
  }
  if (hasUsage) {
    return CHAT.usageSummaryShort(usage.totalTokens)
  }
  return CHAT.usageSavedSummaryShort(usage.savedTokens)
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
      usage.cachedTokens,
    )
  } else if (hasUsage) {
    text = CHAT.usageLine(
      usage.promptTokens,
      usage.completionTokens,
      usage.totalTokens,
      usage.cachedTokens,
    )
  } else {
    text = CHAT.usageSavedLine(usage.savedTokens)
  }

  return (
    <details className="msg-usage-details">
      <summary className="msg-usage-summary">{usageSummaryLabel(usage, hasUsage, hasSaved)}</summary>
      <p className="msg-usage" data-testid="usage-line">
        {text}
      </p>
    </details>
  )
}
