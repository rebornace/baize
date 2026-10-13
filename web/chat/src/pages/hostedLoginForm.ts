import type { ConnectorAuth, HostedLoginField, HostedLoginRecipe } from '../api'

export interface HostedLoginDraft {
  enabled: boolean
  loginTool: string
  mode: string
  fieldsText: string
}

export const EMPTY_HOSTED_LOGIN_DRAFT: HostedLoginDraft = {
  enabled: false,
  loginTool: '',
  mode: 'password_form',
  fieldsText: '',
}

/** Serialize fields as one "name|arg|input|label" line each. */
export function formatHostedLoginFields(fields: HostedLoginField[] | undefined): string {
  if (!fields || fields.length === 0) return ''
  return fields
    .map((f) => [f.name, f.arg, f.input || 'text', f.label || ''].join('|'))
    .join('\n')
}

export function parseHostedLoginFields(text: string): HostedLoginField[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const [name = '', arg = '', input = 'text', label = ''] = line.split('|').map((s) => s.trim())
      return {
        name: name || arg,
        arg: arg || name,
        input: input || 'text',
        label: label || undefined,
      }
    })
    .filter((f) => f.name && f.arg)
}

export function hostedLoginToDraft(recipe: HostedLoginRecipe | undefined): HostedLoginDraft {
  if (!recipe) return { ...EMPTY_HOSTED_LOGIN_DRAFT }
  return {
    enabled: Boolean(recipe.enabled),
    loginTool: recipe.login_tool ?? '',
    mode: recipe.mode || 'password_form',
    fieldsText: formatHostedLoginFields(recipe.fields),
  }
}

export function buildHostedLoginFromDraft(draft: HostedLoginDraft): HostedLoginRecipe {
  const fields = parseHostedLoginFields(draft.fieldsText)
  const out: HostedLoginRecipe = {
    enabled: draft.enabled,
    mode: draft.mode.trim() || undefined,
    login_tool: draft.loginTool.trim() || undefined,
  }
  if (fields.length > 0) out.fields = fields
  return out
}

export function mergeAuthWithHostedLogin(
  base: ConnectorAuth | undefined,
  draft: HostedLoginDraft,
): ConnectorAuth {
  const auth: ConnectorAuth = { ...base, mode: base?.mode ?? 'static' }
  if (base?.static) auth.static = { ...base.static }
  if (base?.passthrough) auth.passthrough = { ...base.passthrough }
  if (base?.vault_ref) auth.vault_ref = { ...base.vault_ref }
  if (base?.capture) auth.capture = { ...base.capture }
  auth.hosted_login = buildHostedLoginFromDraft(draft)
  return auth
}
