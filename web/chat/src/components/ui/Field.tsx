import { cloneElement, isValidElement, useId, type ReactElement, type ReactNode } from 'react'

export interface FieldProps {
  label: ReactNode
  htmlFor?: string
  hint?: ReactNode
  error?: ReactNode
  required?: boolean
  className?: string
  children: ReactNode
}

export function Field({ label, htmlFor, hint, error, required, className = '', children }: FieldProps) {
  const autoId = useId()
  const controlId = htmlFor ?? autoId
  const errorId = `${controlId}-error`
  const hintId = `${controlId}-hint`
  const describedBy = error ? errorId : hint ? hintId : undefined

  const control = isValidElement(children)
    ? cloneElement(children as ReactElement<Record<string, unknown>>, {
        id: controlId,
        'aria-invalid': error ? true : undefined,
        'aria-describedby': describedBy,
        invalid: error ? true : undefined,
      })
    : children

  return (
    <div className={`ui-field${className ? ` ${className}` : ''}`}>
      <label className="ui-field-label" htmlFor={controlId}>
        {label}
        {required && (
          <span className="ui-field-required" aria-hidden="true">
            {' '}*
          </span>
        )}
      </label>
      {control}
      {error ? (
        <p className="ui-field-error" id={errorId}>
          {error}
        </p>
      ) : hint ? (
        <p className="ui-field-hint" id={hintId}>
          {hint}
        </p>
      ) : null}
    </div>
  )
}
