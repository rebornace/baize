import { Component, type ErrorInfo, type ReactNode } from 'react'
import { Button } from './ui'
import { useLocale } from '../locale/LocaleContext'

/** Catches route render and lazy-chunk errors so the app never goes blank. */
export class RouteErrorBoundary extends Component<
  { fallback: ReactNode; children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('route render failed', error, info.componentStack)
  }

  render() {
    return this.state.failed ? this.props.fallback : this.props.children
  }
}

export function RouteLoadFailed({ onReload }: { onReload: () => void }) {
  const { strings } = useLocale()
  const copy = strings.ROUTE_ERROR
  return (
    <div className="route-loading route-error" role="alert" data-testid="route-load-failed">
      <div>
        <p>
          <strong>{copy.title}</strong>
        </p>
        <p className="settings-muted">{copy.hint}</p>
        <Button type="button" size="sm" variant="secondary" onClick={onReload}>
          {copy.reload}
        </Button>
      </div>
    </div>
  )
}
