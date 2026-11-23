import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const HTTP_OPERATIONS = new Set([
  'get',
  'put',
  'post',
  'delete',
  'options',
  'head',
  'patch',
  'trace',
])

const READONLY_DESCRIPTION_PREFIX = 'Baize EXT-SoR readonly filter; source version'

/**
 * @param {Record<string, unknown>} input OpenAPI document (JSON)
 * @returns {Record<string, unknown>} copy with paths trimmed to GET-only
 */
export function filterReadonlyOpenAPI(input) {
  const out = structuredClone(input)
  const paths = /** @type {Record<string, Record<string, unknown>>} */ (out.paths ?? {})
  const nextPaths = {}

  for (const [pathKey, pathItem] of Object.entries(paths)) {
    if (!pathItem || typeof pathItem !== 'object') continue
    const filtered = {}
    for (const [key, value] of Object.entries(pathItem)) {
      if (HTTP_OPERATIONS.has(key)) {
        if (key === 'get') filtered.get = value
        continue
      }
      filtered[key] = value
    }
    if (filtered.get != null) nextPaths[pathKey] = filtered
  }

  out.paths = nextPaths
  return out
}

function withReadonlyDescription(spec, sourceVersion) {
  const version = sourceVersion != null && String(sourceVersion).trim() !== '' ? String(sourceVersion) : 'unknown'
  const prefix = `${READONLY_DESCRIPTION_PREFIX} ${version}`
  const info = spec.info && typeof spec.info === 'object' ? { ...spec.info } : {}
  const existing = typeof info.description === 'string' ? info.description : ''
  if (existing.startsWith(READONLY_DESCRIPTION_PREFIX)) {
    info.description = existing
  } else if (existing) {
    info.description = `${prefix}\n\n${existing}`
  } else {
    info.description = prefix
  }
  return { ...spec, info }
}

function runCli() {
  const [inPath, outPath] = process.argv.slice(2)
  if (!inPath || !outPath) {
    console.error('Usage: node filter_readonly_openapi.mjs <in.json> <out.json>')
    process.exit(1)
  }
  const raw = JSON.parse(readFileSync(inPath, 'utf8'))
  const sourceVersion = raw?.info?.version
  const filtered = filterReadonlyOpenAPI(raw)
  const output = withReadonlyDescription(filtered, sourceVersion)
  writeFileSync(outPath, `${JSON.stringify(output, null, 2)}\n`, 'utf8')
}

const entry = process.argv[1] ? path.resolve(process.argv[1]) : ''
const self = path.resolve(fileURLToPath(import.meta.url))
if (entry === self) runCli()
