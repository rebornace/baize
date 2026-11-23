import type { UsageMeta } from '../foldEvents'
import { CHAT } from '../strings'

export type UsageMetaLineProps = {
  usage?: UsageMeta
  /** Wall-clock run duration in milliseconds (thinking + tools + reply). */
  durationMs?: number
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

// UsageMetaLine renders run wall duration and/or token usage under an assistant
// bubble. Duration stays visible; token breakdown stays in a details summary.
export function UsageMetaLine({ usage, durationMs }: UsageMetaLineProps) {
  const hasUsage = Boolean(usage && usage.totalTokens > 0)
  const hasSaved = Boolean(usage && usage.savedTokens > 0)
  const hasDuration = typeof durationMs === 'number' && Number.isFinite(durationMs) && durationMs >= 0
  if (!hasUsage && !hasSaved && !hasDuration) return null

  let usageDetail: string | null = null
  if (usage && (hasUsage || hasSaved)) {
    if (hasUsage && hasSaved) {
      usageDetail = CHAT.usageBothLine(
        usage.promptTokens,
        usage.completionTokens,
        usage.totalTokens,
        usage.savedTokens,
        usage.cachedTokens,
      )
    } else if (hasUsage) {
      usageDetail = CHAT.usageLine(
        usage.promptTokens,
        usage.completionTokens,
        usage.totalTokens,
        usage.cachedTokens,
      )
    } else {
      usageDetail = CHAT.usageSavedLine(usage.savedTokens)
    }
  }

  return (
    <div className="msg-run-meta" data-testid="run-meta">
      {hasDuration ? (
        <p className="msg-duration" data-testid="duration-line">
          {CHAT.replyDuration(durationMs)}
        </p>
      ) : null}
      {usage && usageDetail ? (
        <details className="msg-usage-details">
          <summary className="msg-usage-summary">
            {usageSummaryLabel(usage, hasUsage, hasSaved)}
          </summary>
          <p className="msg-usage" data-testid="usage-line">
            {usageDetail}
          </p>
        </details>
      ) : null}
    </div>
  )
}
