export interface ComingSoonProps {
  title: string
}

export function ComingSoon({ title }: ComingSoonProps) {
  return (
    <div className="settings-section">
      <h1 className="settings-heading">{title}</h1>
      <p className="settings-empty">即将接入，不会在此填写假配置。</p>
    </div>
  )
}