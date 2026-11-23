import type { HTMLAttributes, ReactNode } from 'react'

export type BadgeTone = 'neutral' | 'success' | 'warning' | 'danger' | 'info'

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: BadgeTone
  children?: ReactNode
}

export function Badge({ tone = 'neutral', className = '', children, ...rest }: BadgeProps) {
  return (
    <span
      className={`ui-badge ${tone} ${className}`.trim()}
      data-testid="ui-badge"
      {...rest}
    >
      {children}
    </span>
  )
}
