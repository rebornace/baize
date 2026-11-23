import type { ReactNode } from 'react'

export interface EmptyStateProps {
  icon?: ReactNode
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
}

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <div className="ui-empty-state" data-testid="ui-empty-state">
      {icon && (
        <div className="ui-empty-state-icon" aria-hidden="true">
          {icon}
        </div>
      )}
      <p className="ui-empty-state-title">{title}</p>
      {description && <p className="ui-empty-state-desc">{description}</p>}
      {action && <div className="ui-empty-state-action">{action}</div>}
    </div>
  )
}
