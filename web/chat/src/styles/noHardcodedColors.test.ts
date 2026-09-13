// web/chat/src/styles/noHardcodedColors.test.ts
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const stylesDir = dirname(fileURLToPath(import.meta.url))
const ALLOW_FILES = new Set(['tokens.css'])

/** Strip block and line comments so sample hex in comments does not fail the scan. */
function stripComments(src: string): string {
  return src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')
}

/** 允许的透明渐变写法（整段匹配后从扫描文本中剔除）。 */
const GRADIENT_ALLOW = [
  /linear-gradient\([^;{}]*transparent[^;{}]*\)/gi,
  /radial-gradient\([^;{}]*transparent[^;{}]*\)/gi,
]

function scanable(src: string): string {
  let s = stripComments(src)
  for (const re of GRADIENT_ALLOW) s = s.replace(re, '')
  return s
}

const COLOR_RE = /#(?:[0-9a-fA-F]{3,8})\b|\brgba?\(/g

describe('no hardcoded colors outside tokens.css', () => {
  it('CSS modules only use tokens (except tokens.css and allowed gradients)', () => {
    const files = readdirSync(stylesDir).filter((f) => f.endsWith('.css'))
    const violations: string[] = []
    for (const file of files) {
      if (ALLOW_FILES.has(file)) continue
      const text = scanable(readFileSync(join(stylesDir, file), 'utf8'))
      const hits = text.match(COLOR_RE)
      if (hits?.length) violations.push(`${file}: ${[...new Set(hits)].join(', ')}`)
    }
    expect(violations, violations.join('\n')).toEqual([])
  })
})
