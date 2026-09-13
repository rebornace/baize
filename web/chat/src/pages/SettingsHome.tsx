import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Lock, RefreshCw } from 'lucide-react'
import { useGate } from '../gateContext'
import { Card, Badge, Spinner } from '../components/ui'
import { SETTINGS_GROUPS, settingsNavItems, type SettingsNavItem } from '../settingsNav'
import { useSettingsBadges } from '../settingsHomeBadges'

export function SettingsHome() {
  const { role } = useGate()
  const navigate = useNavigate()
  const [refreshKey, setRefreshKey] = useState(0)
  const [refreshing, setRefreshing] = useState(false)
  const items = settingsNavItems(role)
  const badges = useSettingsBadges(items, role, refreshKey)

  const refresh = () => {
    setRefreshing(true)
    setRefreshKey((k) => k + 1)
  }

  // Keep the spinner visible for a minimum of 600ms; the timer is owned by an
  // effect so it is cleared on unmount (no setState after unmount) and rapid
  // clicks collapse into a single pending timer.
  useEffect(() => {
    if (!refreshing) return
    const t = window.setTimeout(() => setRefreshing(false), 600)
    return () => window.clearTimeout(t)
  }, [refreshing])

  // Completion bar driven by the same models data; null while loading.
  const modelCount = badges.models === undefined ? null : (badges.models?.tone === 'warning' ? 0 : 1)
  const showOnboard = modelCount === 0

  const renderCard = (item: SettingsNavItem) => {
    const isLocked = role === 'operator' && item.operator === 'locked'
    const badge = isLocked
      ? <span className="settings-locked-label"><Lock size={12} aria-hidden="true" />仅管理员</span>
      : item.badge
        ? (badges[item.badge] === undefined
            ? <Spinner />
            : badges[item.badge]
              ? <Badge tone={badges[item.badge]!.tone}>{badges[item.badge]!.text}</Badge>
              : null)
        : null
    const Icon = item.icon
    return (
      <Card
        key={item.to}
        className={`settings-home-card${isLocked ? ' settings-card-locked' : ''}`}
        icon={<Icon size={20} aria-hidden="true" />}
        title={item.label}
        description={item.desc}
        trailing={<span className="settings-card-trailing">{badge}</span>}
        onClick={isLocked ? undefined : () => navigate(item.to)}
      />
    )
  }

  return (
    <div className="settings-home">
      <div className="settings-home-head">
        <div>
          <h1 className="settings-home-title">设置</h1>
          <p className="settings-home-sub">管理助手的模型、能力和对外连接</p>
        </div>
        <button
          type="button"
          className={`btn ghost sm settings-refresh-btn${refreshing ? ' spinning' : ''}`}
          onClick={refresh}
        >
          <RefreshCw size={14} aria-hidden="true" /> 刷新状态
        </button>
      </div>

      {showOnboard && (
        <div className="settings-onboard" role="status">
          <span>
            {role === 'admin'
              ? '先添加一个模型，助手才能开始对话。'
              : '还没有可用模型，请联系管理员添加。'}
          </span>
          {role === 'admin' && (
            <Link to="/settings/models" className="btn primary sm">去添加</Link>
          )}
        </div>
      )}

      {SETTINGS_GROUPS.map((group) => {
        const groupItems = items.filter((i) => i.group === group.id)
        return (
          <section className="settings-home-group" key={group.id}>
            <h2 className="settings-group-title">{group.label}</h2>
            <div className="settings-card-grid">{groupItems.map(renderCard)}</div>
          </section>
        )
      })}
    </div>
  )
}
