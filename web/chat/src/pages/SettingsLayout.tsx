import { NavLink, Outlet, Link } from 'react-router-dom'

const NAV = [
  { to: '/settings/tools', label: 'Tools' },
  { to: '/settings/identities', label: '账号' },
  { to: '/settings/mcp', label: 'MCP' },
  { to: '/settings/plugins', label: '插件' },
] as const

export function SettingsLayout() {
  return (
    <div className="settings-shell">
      <aside className="settings-nav" aria-label="设置导航">
        <p className="settings-nav-title">设置</p>
        <nav className="settings-nav-list">
          {NAV.map((item) => (
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