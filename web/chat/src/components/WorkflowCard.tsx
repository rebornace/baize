import { Check, Loader2, X } from 'lucide-react'
import type { ChatBlock, WorkflowStepStatus } from '../foldEvents'
import { WORKFLOW_PREPARING } from '../strings'

type WorkflowBlock = Extract<ChatBlock, { kind: 'workflow' }>

export interface WorkflowCardProps {
  block: WorkflowBlock
}

export function WorkflowCard({ block }: WorkflowCardProps) {
  const total = block.steps.length
  const current = block.steps.filter(
    (s) => s.status === 'done' || s.status === 'running',
  ).length
  return (
    <div className="tool-card workflow-card">
      <div className="tool-card-header workflow-card-header">
        <span className="workflow-progress">
          {total === 0 ? WORKFLOW_PREPARING : `第 ${Math.max(current, 1)} / 共 ${total} 步`}
        </span>
      </div>
      <ol className="workflow-steps">
        {block.steps.map((step, i) => (
          <li
            key={step.id}
            className={`workflow-step workflow-step-${step.status}`}
            data-status={step.status}
          >
            <StepIcon status={step.status} />
            <span>步骤 {i + 1}</span>
          </li>
        ))}
      </ol>
    </div>
  )
}

function StepIcon({ status }: { status: WorkflowStepStatus }) {
  if (status === 'running') return <Loader2 size={13} className="icon-spin" aria-hidden />
  if (status === 'failed') return <X size={13} aria-hidden />
  if (status === 'done') return <Check size={13} aria-hidden />
  return <span className="workflow-step-dot" aria-hidden />
}
