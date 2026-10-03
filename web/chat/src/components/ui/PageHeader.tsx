import type { ReactNode } from 'react'

export interface PageHeaderProps {
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
}

export function PageHeader({ title, description, actions }: PageHeaderProps) {
  return (
    <header className="ui-page-header">
      <div className="ui-page-header-row">
        <h1 className="ui-page-header-title">{title}</h1>
        {actions && <div className="ui-page-header-actions">{actions}</div>}
      </div>
      {description && <p className="ui-page-header-desc">{description}</p>}
    </header>
  )
}
