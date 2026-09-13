import type { ButtonHTMLAttributes, ReactNode } from 'react'

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'
export type ButtonSize = 'md' | 'sm'

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: ButtonSize
  children?: ReactNode
}

export function Button({
  variant = 'secondary',
  size = 'md',
  className = '',
  type = 'button',
  children,
  ...rest
}: ButtonProps) {
  // size 放在 variant 之前，保证默认 variant 下类名含连续的 "btn sm"（测试即规格）
  const classes = ['btn', size === 'sm' ? 'sm' : '', variant, className]
    .filter(Boolean)
    .join(' ')
  return (
    <button type={type} className={classes} data-testid="ui-button" {...rest}>
      {children}
    </button>
  )
}
