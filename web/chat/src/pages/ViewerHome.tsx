import { PageHeader } from '../components/ui'
import { useGate } from '../gateContext'
import { CHAT } from '../strings'
import { AuditLogCard } from './AuditLogCard'
import { CostSummarySection } from './models/CostSummarySection'

/** Read-only dashboard for the Viewer role: costs and audit, no controls. */
export function ViewerHome() {
  const { viewerWorkspace } = useGate()
  if (viewerWorkspace) {
    return (
      <div className="settings-panel">
        <PageHeader title={CHAT.viewerDashboard} description={CHAT.viewerDashboardDesc} />
        <p className="settings-muted">{CHAT.viewerScoped(viewerWorkspace)}</p>
      </div>
    )
  }
  return (
    <div className="settings-panel">
      <PageHeader title={CHAT.viewerDashboard} description={CHAT.viewerDashboardDesc} />
      <CostSummarySection readOnly />
      <AuditLogCard readOnly />
    </div>
  )
}
