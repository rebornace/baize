import { NavLink, Outlet, Link } from 'react-router-dom'
import { Menu } from 'lucide-react'
import { ThemeToggle } from '../components/ui'
import { useGate } from '../gateContext'
import { settingsNavItems } from '../settingsNav'
import { useDrawer } from '../useDrawer'

export function SettingsLayout() {
  const { role } = useGate()
  const nav = settingsNavItems(role)
  const drawer = useDrawer()
  return (
    <div className={`settings-shell app-with-drawer${drawer.isOpen ? ' drawer-open' : ''}`}>
      <button
        type="button"
        className="app-drawer-scrim"
        aria-label="关闭菜单"
        onClick={drawer.close}
      />
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
              onClick={drawer.close}
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        <ThemeToggle />
        <Link to="/" className="settings-back">
          返回聊天
        </Link>
      </aside>
      <main className="settings-main">
        <div className="app-mobile-bar">
          <button
            type="button"
            className="app-menu-btn"
            aria-label="打开设置菜单"
            onClick={drawer.open}
          >
            <Menu size={20} aria-hidden="true" />
          </button>
          <strong>设置</strong>
        </div>
        <Outlet />
      </main>
    </div>
  )
}
