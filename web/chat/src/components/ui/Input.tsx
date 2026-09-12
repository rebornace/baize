import type { InputHTMLAttributes } from 'react'

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  invalid?: boolean
}

export function Input({ invalid, className = '', ...rest }: InputProps) {
  const invalidCls = invalid ? ' invalid' : ''
  return <input className={`ui-input${invalidCls}${className ? ` ${className}` : ''}`} {...rest} />
}
