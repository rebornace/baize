import { NavLink, Outlet, Link } from 'react-router-dom'
import { useGate } from '../gateContext'
import { settingsNavItems } from '../settingsNav'

export function SettingsLayout() {
  const { role } = useGate()
  const nav = settingsNavItems(role)
  return (
    <div className="settings-shell">
      <aside className="settings-nav" aria-label="设置导航">
        <p className="settings-nav-title">设置</p>
        <nav className="settings-nav-list">
          {nav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                `settings-nav-link${isActive ? ' active' : ''}`
              }
              end
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        <Link to="/" className="settings-back">
          返回聊天
        </Link>
      </aside>
      <main className="settings-main">
        <Outlet />
      </main>
    </div>
  )
}
