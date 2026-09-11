import type { SelectHTMLAttributes } from 'react'

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  invalid?: boolean
}

export function Select({ invalid, className = '', children, ...rest }: SelectProps) {
  const invalidCls = invalid ? ' invalid' : ''
  return (
    <select className={`ui-select${invalidCls}${className ? ` ${className}` : ''}`} {...rest}>
      {children}
    </select>
  )
}
