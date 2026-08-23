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
