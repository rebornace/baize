import type { TextareaHTMLAttributes } from 'react'

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  invalid?: boolean
}

export function Textarea({ invalid, className = '', ...rest }: TextareaProps) {
  const invalidCls = invalid ? ' invalid' : ''
  return <textarea className={`ui-textarea${invalidCls}${className ? ` ${className}` : ''}`} {...rest} />
}
