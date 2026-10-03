import { COMING_SOON } from '../strings'

export interface ComingSoonProps {
  title: string
}

export function ComingSoon({ title }: ComingSoonProps) {
  return (
    <div className="settings-section">
      <h1 className="settings-heading">{title}</h1>
      <p className="settings-empty">{COMING_SOON.body}</p>
    </div>
  )
}
