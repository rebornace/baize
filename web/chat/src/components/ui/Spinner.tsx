export interface SpinnerProps {
  label?: string
  className?: string
}

export function Spinner({ label, className = '' }: SpinnerProps) {
  return (
    <span className={`ui-spinner ${className}`.trim()} role="status" aria-label={label} data-testid="ui-spinner">
      <span className="ui-spinner-dot" aria-hidden="true" />
      {label && <span className="ui-spinner-label">{label}</span>}
    </span>
  )
}
