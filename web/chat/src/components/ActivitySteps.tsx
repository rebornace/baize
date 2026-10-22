import { useState, type ReactNode } from 'react'
import { Check, ChevronDown, Loader2, X } from 'lucide-react'
import {
  activityStepsNeedAttention,
  focusActivityStepIndex,
  shouldCollapseActivitySteps,
  type ActivityStepBlock,
} from '../activitySteps'
import { toolPhrase, type ToolCatalog } from '../friendlyTool'
import { CHAT, WORKFLOW, WORKFLOW_PREPARING } from '../strings'

export type ActivityStepsProps = {
  blocks: ActivityStepBlock[]
  catalog: ToolCatalog
  children: ReactNode
}

function stepLabel(block: ActivityStepBlock, catalog: ToolCatalog): string {
  if (block.kind === 'tool') {
    return toolPhrase(block.name, block.status, catalog)
  }
  const total = block.steps.length
  if (total === 0) return WORKFLOW_PREPARING
  const current = block.steps.filter((s) => s.status === 'done' || s.status === 'running').length
  return WORKFLOW.progress(Math.max(current, 1), total)
}

function StepStatusIcon({ block }: { block: ActivityStepBlock }) {
  if (block.kind === 'workflow') {
    if (block.steps.some((s) => s.status === 'running')) {
      return <Loader2 size={15} className="icon-spin" aria-hidden />
    }
    if (block.steps.some((s) => s.status === 'failed')) {
      return <X size={15} aria-hidden />
    }
    if (block.steps.length > 0 && block.steps.every((s) => s.status === 'done')) {
      return <Check size={15} aria-hidden />
    }
    return <Loader2 size={15} className="icon-spin" aria-hidden />
  }
  if (block.status === 'running' || block.status === 'approved') {
    return <Loader2 size={15} className="icon-spin" aria-hidden />
  }
  if (block.status === 'failed' || block.status === 'rejected') {
    return <X size={15} aria-hidden />
  }
  if (block.status === 'waiting_human') {
    return <Loader2 size={15} className="icon-spin" aria-hidden />
  }
  return <Check size={15} aria-hidden />
}

/**
 * Many tool/workflow cards collapse into one strip by default.
 * Always start collapsed; only the user can expand/collapse.
 */
export function ActivitySteps({ blocks, catalog, children }: ActivityStepsProps) {
  const focusIdx = focusActivityStepIndex(blocks)
  const focus = focusIdx >= 0 ? blocks[focusIdx] : undefined
  const needsAttention = activityStepsNeedAttention(blocks)
  const [expanded, setExpanded] = useState(false)

  if (!shouldCollapseActivitySteps(blocks)) {
    return <>{children}</>
  }

  const progressLabel = CHAT.activityProgress(Math.max(focusIdx + 1, 1), blocks.length)
  const focusText = focus ? stepLabel(focus, catalog) : ''

  return (
    <div
      className={`activity-steps${expanded ? ' is-expanded' : ' is-collapsed'}${
        needsAttention ? ' needs-attention' : ''
      }`}
      data-testid="activity-steps"
      data-expanded={expanded ? 'true' : 'false'}
    >
      <button
        type="button"
        className="activity-steps-toggle"
        aria-expanded={expanded}
        onClick={() => setExpanded((v) => !v)}
      >
        <span className="activity-steps-summary">
          <span className="activity-steps-progress">{progressLabel}</span>
          {focus && (
            <span className="activity-steps-focus" key={`${focusIdx}-${focusText}`}>
              <StepStatusIcon block={focus} />
              <span className="activity-steps-focus-text">{focusText}</span>
            </span>
          )}
          {needsAttention && !expanded ? (
            <span className="activity-steps-attention">{CHAT.activityNeedsAttention}</span>
          ) : null}
        </span>
        <span className="activity-steps-toggle-label">
          {expanded ? CHAT.activityCollapse : CHAT.activityExpand}
          <ChevronDown
            size={14}
            className={`activity-steps-chevron${expanded ? ' expanded' : ''}`}
            aria-hidden
          />
        </span>
      </button>
      {expanded ? <div className="activity-steps-body">{children}</div> : null}
    </div>
  )
}
