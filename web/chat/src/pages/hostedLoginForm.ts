import type {
  ConnectorAuth,
  HostedLoginConfig,
  HostedLoginField,
  HostedLoginMethod,
  HostedLoginOAuth,
} from '../api'

export interface HostedLoginMethodDraft {
  id: string
  label: string
  mode: string
  role: string
  identityIds: string[]
  loginTool: string
  fieldsText: string
  oauthAuthorizeURL: string
  oauthTokenURL: string
  oauthClientID: string
  oauthClientSecret: string
  oauthScopes: string
}

export interface HostedLoginDraft {
  enabled: boolean
  defaultMethodId: string
  methods: HostedLoginMethodDraft[]
}

export const EMPTY_HOSTED_LOGIN_METHOD_DRAFT: HostedLoginMethodDraft = {
  id: '',
  label: '',
  mode: 'password_form',
  role: '',
  identityIds: [],
  loginTool: '',
  fieldsText: '',
  oauthAuthorizeURL: '',
  oauthTokenURL: '',
  oauthClientID: '',
  oauthClientSecret: '',
  oauthScopes: '',
}

/** Slot role for a method draft: explicit role, else method id. */
export function methodDraftEffectiveRole(draft: HostedLoginMethodDraft): string {
  return draft.role.trim() || draft.id.trim()
}

export const EMPTY_HOSTED_LOGIN_DRAFT: HostedLoginDraft = {
  enabled: false,
  defaultMethodId: '',
  methods: [],
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

function methodToDraft(method: HostedLoginMethod): HostedLoginMethodDraft {
  const oauth = method.oauth
  return {
    id: method.id ?? '',
    label: method.label ?? '',
    mode: method.mode || 'password_form',
    role: method.role ?? '',
    identityIds: [...(method.identity_ids ?? [])].map((s) => s.trim()).filter(Boolean),
    loginTool: method.login_tool ?? '',
    fieldsText: formatHostedLoginFields(method.fields),
    oauthAuthorizeURL: oauth?.authorize_url ?? '',
    oauthTokenURL: oauth?.token_url ?? '',
    oauthClientID: oauth?.client_id ?? '',
    oauthClientSecret: oauth?.client_secret ?? '',
    oauthScopes: (oauth?.scopes ?? []).join(' '),
  }
}

function buildOAuthFromMethodDraft(draft: HostedLoginMethodDraft): HostedLoginOAuth | undefined {
  if (draft.mode !== 'oauth') return undefined
  const scopes = draft.oauthScopes
    .split(/\s+/)
    .map((s) => s.trim())
    .filter(Boolean)
  const oauth: HostedLoginOAuth = {
    authorize_url: draft.oauthAuthorizeURL.trim() || undefined,
    token_url: draft.oauthTokenURL.trim() || undefined,
    client_id: draft.oauthClientID.trim() || undefined,
    client_secret: draft.oauthClientSecret.trim() || undefined,
    scopes: scopes.length > 0 ? scopes : undefined,
  }
  if (
    oauth.authorize_url ||
    oauth.token_url ||
    oauth.client_id ||
    oauth.client_secret ||
    oauth.scopes
  ) {
    return oauth
  }
  return undefined
}

const passwordArgRE = /pass|pwd|secret|credential|passwd|口令|密码/i

function fieldsHavePassword(fields: HostedLoginField[]): boolean {
  return fields.some(
    (f) =>
      String(f.input || '').toLowerCase() === 'password' ||
      passwordArgRE.test(f.arg || '') ||
      passwordArgRE.test(f.name || ''),
  )
}

/** Ensure password_form drafts include a password field (matches server EnsurePasswordFormFields). */
function ensurePasswordFormFields(
  mode: string,
  fields: HostedLoginField[],
): HostedLoginField[] {
  const m = mode.trim() || 'password_form'
  if (m !== 'password_form') return fields
  if (fieldsHavePassword(fields)) {
    return fields.map((f) =>
      passwordArgRE.test(f.arg || '') || passwordArgRE.test(f.name || '')
        ? { ...f, input: 'password' }
        : f,
    )
  }
  if (fields.length === 0) {
    return [
      { name: 'phone', arg: 'phone', input: 'text', label: '账号/手机号' },
      { name: 'password', arg: 'password', input: 'password', label: '密码' },
    ]
  }
  return [
    ...fields,
    { name: 'password', arg: 'password', input: 'password', label: 'Password' },
  ]
}

function buildMethodFromDraft(draft: HostedLoginMethodDraft): HostedLoginMethod | undefined {
  const id = draft.id.trim()
  if (!id) return undefined
  const mode = draft.mode.trim()
  const fields = ensurePasswordFormFields(mode, parseHostedLoginFields(draft.fieldsText))
  const out: HostedLoginMethod = { id }
  const label = draft.label.trim()
  const role = draft.role.trim()
  const loginTool = draft.loginTool.trim()
  const identityIds = [...new Set(draft.identityIds.map((s) => s.trim()).filter(Boolean))]
  if (label) out.label = label
  if (mode) out.mode = mode
  if (role) out.role = role
  if (identityIds.length > 0) out.identity_ids = identityIds
  if (loginTool) out.login_tool = loginTool
  if (fields.length > 0) out.fields = fields
  const oauth = buildOAuthFromMethodDraft(draft)
  if (oauth) out.oauth = oauth
  return out
}

function methodDraftIsMeaningful(draft: HostedLoginMethodDraft): boolean {
  return (
    Boolean(draft.id.trim()) ||
    Boolean(draft.label.trim()) ||
    Boolean(draft.loginTool.trim()) ||
    parseHostedLoginFields(draft.fieldsText).length > 0 ||
    Boolean(buildOAuthFromMethodDraft(draft))
  )
}

export function hostedLoginToDraft(config: HostedLoginConfig | undefined): HostedLoginDraft {
  if (!config) return { ...EMPTY_HOSTED_LOGIN_DRAFT, methods: [] }
  const methods = (config.methods ?? []).map(methodToDraft)
  return {
    enabled: Boolean(config.enabled),
    defaultMethodId: config.default_method_id ?? methods[0]?.id ?? '',
    methods,
  }
}

export function buildHostedLoginFromDraft(draft: HostedLoginDraft): HostedLoginConfig {
  const methods = draft.methods
    .map(buildMethodFromDraft)
    .filter((m): m is HostedLoginMethod => m !== undefined)
  const out: HostedLoginConfig = {
    enabled: draft.enabled,
  }
  if (methods.length > 0) out.methods = methods
  const defaultId = draft.defaultMethodId.trim() || methods[0]?.id
  if (defaultId) out.default_method_id = defaultId
  return out
}

function configIsMeaningful(config: HostedLoginConfig): boolean {
  if (config.enabled) return true
  for (const m of config.methods ?? []) {
    if (
      Boolean(m.login_tool) ||
      (m.fields?.length ?? 0) > 0 ||
      Boolean(m.oauth) ||
      Boolean(m.mode) ||
      Boolean(m.label)
    ) {
      return true
    }
  }
  return false
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
  const config = buildHostedLoginFromDraft(draft)
  if (configIsMeaningful(config)) {
    auth.hosted_login = config
  } else if (base?.hosted_login) {
    // Explicitly disable so an edit can clear a previously saved config.
    auth.hosted_login = { enabled: false }
  } else {
    delete auth.hosted_login
  }
  return auth
}

/** Whether a method draft row carries user-edited content (wizard helpers). */
export function hostedLoginMethodDraftIsMeaningful(draft: HostedLoginMethodDraft): boolean {
  return methodDraftIsMeaningful(draft)
}

function primaryMethodIndex(draft: HostedLoginDraft): number {
  if (draft.methods.length === 0) return -1
  const defaultId = draft.defaultMethodId.trim()
  if (defaultId) {
    const idx = draft.methods.findIndex((m) => m.id === defaultId)
    if (idx >= 0) return idx
  }
  return 0
}

/** Ensure at least one method row exists (legacy connector editor single-method UI). */
export function ensurePrimaryMethodDraft(draft: HostedLoginDraft): HostedLoginDraft {
  if (draft.methods.length > 0) return draft
  const method: HostedLoginMethodDraft = { ...EMPTY_HOSTED_LOGIN_METHOD_DRAFT, id: 'default' }
  return { ...draft, methods: [method], defaultMethodId: method.id }
}

export function primaryMethodDraft(draft: HostedLoginDraft): HostedLoginMethodDraft {
  const normalized = ensurePrimaryMethodDraft(draft)
  const idx = primaryMethodIndex(normalized)
  return normalized.methods[idx] ?? normalized.methods[0]
}

export function patchPrimaryMethodDraft(
  draft: HostedLoginDraft,
  patch: Partial<HostedLoginMethodDraft>,
): Partial<HostedLoginDraft> {
  const normalized = ensurePrimaryMethodDraft(draft)
  const idx = primaryMethodIndex(normalized)
  const methods = normalized.methods.map((m, i) => (i === idx ? { ...m, ...patch } : m))
  const defaultMethodId = normalized.defaultMethodId.trim() || methods[idx]?.id || ''
  return { methods, defaultMethodId }
}

export function methodDraftFromCandidate(method: HostedLoginMethod): HostedLoginMethodDraft {
  return methodToDraft(method)
}
