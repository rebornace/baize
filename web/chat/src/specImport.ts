export type ImportFormat = 'auto' | 'openapi3' | 'swagger2' | 'postman'

export type DetectedImportFormat = Exclude<ImportFormat, 'auto'>

function rootLooksLikeSwagger2(root: Record<string, unknown>): boolean {
  const v = root.swagger
  return typeof v === 'string' && v.startsWith('2')
}

function rootLooksLikeOpenAPI3(root: Record<string, unknown>): boolean {
  const v = root.openapi
  return typeof v === 'string' && v.startsWith('3')
}

function rootLooksLikePostman(root: Record<string, unknown>): boolean {
  const info = root.info
  if (!info || typeof info !== 'object' || Array.isArray(info)) return false
  const schema = (info as Record<string, unknown>).schema
  return typeof schema === 'string' && schema.toLowerCase().includes('postman')
}

/** Lightweight client-side pre-detection (JSON only; aligns with server DetectFormat). */
export function detectImportFormat(content: string): DetectedImportFormat | null {
  const trimmed = content.trim()
  if (trimmed === '' || (trimmed[0] !== '{' && trimmed[0] !== '[')) {
    return null
  }
  let root: unknown
  try {
    root = JSON.parse(trimmed)
  } catch {
    return null
  }
  if (!root || typeof root !== 'object' || Array.isArray(root)) {
    return null
  }
  const obj = root as Record<string, unknown>
  if (rootLooksLikeSwagger2(obj)) return 'swagger2'
  if (rootLooksLikeOpenAPI3(obj)) return 'openapi3'
  if (rootLooksLikePostman(obj)) return 'postman'
  return null
}

export function importFormatLabel(format: string | undefined): string {
  switch (format) {
    case 'openapi3':
      return 'OpenAPI 3'
    case 'swagger2':
      return 'Swagger 2'
    case 'postman':
      return 'Postman Collection'
    case 'auto':
      return '自动识别'
    case '':
    case undefined:
      return '—'
    default:
      return format
  }
}

export function detectedFormatHint(format: DetectedImportFormat | null): string {
  if (!format) return '未能自动识别格式，请选择 import_format 或换 OpenAPI/Postman 导出'
  switch (format) {
    case 'openapi3':
      return '识别为 OpenAPI 3'
    case 'swagger2':
      return '识别为 Swagger 2，将转换为 OpenAPI 3'
    case 'postman':
      return '识别为 Postman Collection，将转换为 OpenAPI 3'
    default: {
      const _exhaustive: never = format
      return _exhaustive
    }
  }
}
