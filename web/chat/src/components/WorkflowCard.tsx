import type { ChatBlock, WorkflowStepStatus } from '../foldEvents'

type WorkflowBlock = Extract<ChatBlock, { kind: 'workflow' }>

const STEP_LABEL: Record<WorkflowStepStatus, string> = {
  pending: '待执行',
  running: '执行中',
  done: '完成',
  failed: '失败',
}

export interface WorkflowCardProps {
  block: WorkflowBlock
}

export function WorkflowCard({ block }: WorkflowCardProps) {
  return (
    <div className="tool-card workflow-card">
      <div className="tool-card-header workflow-card-header">
        <span className="tool-card-name">工作流 · {block.skill}</span>
      </div>
      <ol className="workflow-steps">
        {block.steps.map((step) => (
          <li
            key={step.id}
            className={`workflow-step workflow-step-${step.status}`}
            data-status={step.status}
          >
            <span className="workflow-step-id">{step.id}</span>
            <span className="workflow-step-status">{STEP_LABEL[step.status]}</span>
          </li>
        ))}
      </ol>
    </div>
  )
}
