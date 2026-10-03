import { NavLink, Outlet, Link } from 'react-router-dom'
import { Menu } from 'lucide-react'
import { ThemeToggle } from '../components/ui'
import { SidebarResizer } from '../components/SidebarResizer'
import { useGate } from '../gateContext'
import { useLocale } from '../locale/LocaleContext'
import { OVERVIEW_ITEM, SETTINGS_GROUPS, visibleNavItems } from '../settingsNav'
import { useDrawer } from '../useDrawer'

export function SettingsLayout() {
  const { role } = useGate()
  // Subscribe so sidebar labels rebuild when language changes (without full page reload).
  const { strings } = useLocale()
  const navCopy = strings.SETTINGS_NAV
  const nav = visibleNavItems(role)
  const drawer = useDrawer()
  return (
    <div className={`settings-shell app-with-drawer${drawer.isOpen ? ' drawer-open' : ''}`}>
      <button
        type="button"
        className="app-drawer-scrim"
        aria-label={navCopy.closeMenu}
        onClick={drawer.close}
      />
      <aside className="settings-nav" aria-label={navCopy.navAria}>
        <p className="settings-nav-title">{navCopy.title}</p>
        <nav className="settings-nav-list">
          <NavLink
            to={OVERVIEW_ITEM.to}
            end
            className={({ isActive }) =>
              `settings-nav-link settings-nav-overview${isActive ? ' active' : ''}`
            }
            onClick={drawer.close}
          >
            {strings.SETTINGS_NAV.overview}
          </NavLink>

          {SETTINGS_GROUPS.map((group) => {
            const items = nav.filter((i) => i.group === group.id)
            if (items.length === 0) return null
            return (
              <div className="settings-nav-group" key={group.id}>
                <p className="settings-nav-group-title">{group.label}</p>
                {items.map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    className={({ isActive }) =>
                      `settings-nav-link${isActive ? ' active' : ''}`
                    }
                    onClick={drawer.close}
                  >
                    {item.label}
                  </NavLink>
                ))}
              </div>
            )
          })}
        </nav>
        <div className="settings-nav-bottom">
          <ThemeToggle />
          <Link to="/" className="settings-back">
            {navCopy.backToChat}
          </Link>
        </div>
        <SidebarResizer />
      </aside>
      <main className="settings-main">
        <div className="app-mobile-bar">
          <button
            type="button"
            className="app-menu-btn"
            aria-label={navCopy.openMenu}
            onClick={drawer.open}
          >
            <Menu size={20} aria-hidden="true" />
          </button>
          <strong>{navCopy.title}</strong>
        </div>
        <Outlet />
      </main>
    </div>
  )
}
