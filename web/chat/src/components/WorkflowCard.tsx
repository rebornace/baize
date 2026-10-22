import { useState } from 'react'
import { Check, ChevronDown, Loader2, X } from 'lucide-react'
import type { ChatBlock, WorkflowStepStatus } from '../foldEvents'
import { WORKFLOW, WORKFLOW_PREPARING } from '../strings'

type WorkflowBlock = Extract<ChatBlock, { kind: 'workflow' }>

export interface WorkflowCardProps {
  block: WorkflowBlock
}

function focusStepIndex(steps: WorkflowBlock['steps']): number {
  if (steps.length === 0) return -1
  const running = steps.findIndex((s) => s.status === 'running')
  if (running >= 0) return running
  const failed = steps.findIndex((s) => s.status === 'failed')
  if (failed >= 0) return failed
  for (let i = steps.length - 1; i >= 0; i--) {
    if (steps[i].status === 'done') return i
  }
  return 0
}

export function WorkflowCard({ block }: WorkflowCardProps) {
  const total = block.steps.length
  const current = block.steps.filter(
    (s) => s.status === 'done' || s.status === 'running',
  ).length
  const focusIdx = focusStepIndex(block.steps)
  const [expanded, setExpanded] = useState(false)
  const progress =
    total === 0
      ? WORKFLOW_PREPARING
      : WORKFLOW.progress(Math.max(current, 1), total)

  // Single-step workflows stay compact; no expand chrome.
  if (total <= 1) {
    return (
      <div className="tool-card workflow-card" data-testid="workflow-card">
        <div className="tool-card-header workflow-card-header">
          <span className="workflow-progress">{progress}</span>
          {total === 1 ? (
            <span className="workflow-focus-step">
              <StepIcon status={block.steps[0].status} />
              <span>{WORKFLOW.step(1)}</span>
            </span>
          ) : null}
        </div>
      </div>
    )
  }

  return (
    <div
      className={`tool-card workflow-card${expanded ? ' is-expanded' : ' is-collapsed'}`}
      data-testid="workflow-card"
    >
      <button
        type="button"
        className="tool-card-header workflow-card-header"
        onClick={() => setExpanded((v) => !v)}
        aria-expanded={expanded}
      >
        <span className="workflow-header-main">
          <span className="workflow-progress">{progress}</span>
          {!expanded && focusIdx >= 0 ? (
            <span className="workflow-focus-step" key={focusIdx}>
              <StepIcon status={block.steps[focusIdx].status} />
              <span>{WORKFLOW.step(focusIdx + 1)}</span>
            </span>
          ) : null}
        </span>
        <span className="workflow-toggle-label">
          {expanded ? WORKFLOW.collapse : WORKFLOW.expand}
          <ChevronDown
            size={14}
            className={`tool-card-chevron${expanded ? ' expanded' : ''}`}
            aria-hidden
          />
        </span>
      </button>
      {expanded ? (
        <ol className="workflow-steps">
          {block.steps.map((step, i) => (
            <li
              key={step.id}
              className={`workflow-step workflow-step-${step.status}`}
              data-status={step.status}
            >
              <StepIcon status={step.status} />
              <span>{WORKFLOW.step(i + 1)}</span>
            </li>
          ))}
        </ol>
      ) : null}
    </div>
  )
}

function StepIcon({ status }: { status: WorkflowStepStatus }) {
  if (status === 'running') return <Loader2 size={13} className="icon-spin" aria-hidden />
  if (status === 'failed') return <X size={13} aria-hidden />
  if (status === 'done') return <Check size={13} aria-hidden />
  return <span className="workflow-step-dot" aria-hidden />
}
