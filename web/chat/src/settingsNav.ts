export type SettingsRole = 'operator' | 'admin'

export interface SettingsNavItem {
  to: string
  label: string
}

export function settingsNavItems(role: SettingsRole): SettingsNavItem[] {
  switch (role) {
    case 'operator':
      return [{ to: '/settings/identities', label: '账号' }]
    case 'admin':
      return [
        { to: '/settings/tools', label: 'Tools' },
        { to: '/settings/openapi', label: 'OpenAPI' },
        { to: '/settings/skills', label: 'Skills' },
        { to: '/settings/identities', label: '账号' },
        { to: '/settings/mcp', label: 'MCP' },
        { to: '/settings/plugins', label: '插件' },
        { to: '/settings/webhooks', label: 'Webhook' },
        { to: '/settings/inbox', label: 'Inbox' },
        { to: '/settings/channels/weixin', label: '渠道' },
        { to: '/settings/storage', label: '存储' },
      ]
    default: {
      const _exhaustive: never = role
      return _exhaustive
    }
  }
}
