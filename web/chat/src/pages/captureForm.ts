import type { ConnectorAuth } from '../api'

export interface CaptureDraft {
  toolNameGlob: string
  tokenPathsText: string
  labelPathsText: string
  headerTemplate: string
  defaultScheme: string
}

export const EMPTY_CAPTURE_DRAFT: CaptureDraft = {
  toolNameGlob: '',
  tokenPathsText: '',
  labelPathsText: '',
  headerTemplate: '',
  defaultScheme: '',
}

export function formatPathLines(paths: string[] | undefined): string {
  if (!paths || paths.length === 0) return ''
  return paths.join('\n')
}

export function captureToDraft(capture: ConnectorAuth['capture'] | undefined): CaptureDraft {
  if (!capture) return { ...EMPTY_CAPTURE_DRAFT }
  return {
    toolNameGlob: capture.tool_name_glob ?? '',
    tokenPathsText: formatPathLines(capture.token_json_paths),
    labelPathsText: formatPathLines(capture.label_json_paths),
    headerTemplate: capture.header_template ?? '',
    defaultScheme: capture.default_scheme ?? '',
  }
}

export function parsePathLines(text: string): string[] {
  return text
    .split('\n')
    .map((s) => s.trim())
    .filter((s) => s !== '')
}

export function buildCaptureFromDraft(draft: CaptureDraft): ConnectorAuth['capture'] {
  const toolNameGlob = draft.toolNameGlob.trim()
  const headerTemplate = draft.headerTemplate.trim()
  const defaultScheme = draft.defaultScheme.trim()
  const tokenPaths = parsePathLines(draft.tokenPathsText)
  const labelPaths = parsePathLines(draft.labelPathsText)

  const capture: ConnectorAuth['capture'] = {}
  if (toolNameGlob !== '') capture.tool_name_glob = toolNameGlob
  if (tokenPaths.length > 0) capture.token_json_paths = tokenPaths
  if (labelPaths.length > 0) capture.label_json_paths = labelPaths
  if (headerTemplate !== '') capture.header_template = headerTemplate
  if (defaultScheme !== '') capture.default_scheme = defaultScheme
  return capture
}

/** Merge capture draft into existing auth without changing mode / static / passthrough / vault_ref. */
export function mergeAuthWithCapture(
  base: ConnectorAuth | undefined,
  draft: CaptureDraft,
): ConnectorAuth {
  const auth: ConnectorAuth = { ...base, mode: base?.mode ?? 'static' }
  if (base?.static) auth.static = { ...base.static }
  if (base?.passthrough) auth.passthrough = { ...base.passthrough }
  if (base?.vault_ref) auth.vault_ref = { ...base.vault_ref }
  auth.capture = buildCaptureFromDraft(draft)
  return auth
}

export function connectorSupportsLoginCapture(type: string | undefined): boolean {
  return type === 'openapi' || type === 'http'
}

function hasStoredCapture(capture: ConnectorAuth['capture'] | undefined): boolean {
  if (!capture) return false
  if ((capture.tool_name_glob ?? '').trim() === '__none__') return true
  if ((capture.tool_name_glob ?? '').trim() !== '') return true
  if ((capture.token_json_paths?.length ?? 0) > 0) return true
  if ((capture.label_json_paths?.length ?? 0) > 0) return true
  if ((capture.header_template ?? '').trim() !== '') return true
  if ((capture.default_scheme ?? '').trim() !== '') return true
  return false
}

export function captureSummaryLabel(capture: ConnectorAuth['capture'] | undefined): string | null {
  if (!hasStoredCapture(capture)) return null
  const glob = (capture?.tool_name_glob ?? '').trim()
  if (glob === '__none__') return '捕获已关闭'
  if (glob !== '') {
    const shown = glob.length > 32 ? `${glob.slice(0, 29)}…` : glob
    return `捕获 ${shown}`
  }
  return '捕获（默认 *login*）'
}

export function mergeAuthPreserveCapture(
  built: ConnectorAuth,
  existing: ConnectorAuth | undefined,
): ConnectorAuth {
  if (!hasStoredCapture(existing?.capture)) return built
  return { ...built, capture: { ...existing!.capture! } }
}
