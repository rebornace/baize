import type { KeyboardEvent, ReactNode } from 'react'

export interface CardProps {
  title?: ReactNode
  description?: ReactNode
  icon?: ReactNode
  /** 右上角附加内容（如状态徽标或箭头）。 */
  trailing?: ReactNode
  className?: string
  onClick?: () => void
  children?: ReactNode
}

export function Card({
  title,
  description,
  icon,
  trailing,
  className = '',
  onClick,
  children,
  ...rest
}: CardProps) {
  const clickable = onClick ? ' ui-card-clickable' : ''
  const handleKeyDown = onClick
    ? (e: KeyboardEvent<HTMLDivElement>) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onClick()
        }
      }
    : undefined
  return (
    <div
      className={`ui-card${clickable} ${className}`.trim()}
      data-testid="ui-card"
      onClick={onClick}
      onKeyDown={handleKeyDown}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      {...rest}
    >
      {(title || trailing) && (
        <div className="ui-card-head">
          <div className="ui-card-title">
            {icon && <span className="ui-card-icon">{icon}</span>}
            {title}
          </div>
          {trailing}
        </div>
      )}
      {description && <p className="ui-card-desc">{description}</p>}
      {children}
    </div>
  )
}
