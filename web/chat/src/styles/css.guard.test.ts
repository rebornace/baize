import { readFileSync, readdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// UI-0 守卫：样式只允许引用 tokens.css 定义的变量与色值。
// 这些规则对应 docs 里的设计约定；新增硬编码或漏定义变量会直接让测试失败。
const dir = dirname(fileURLToPath(import.meta.url))
const files = readdirSync(dir).filter((f) => f.endsWith('.css'))
const read = (name: string) => readFileSync(join(dir, name), 'utf8')
const stripComments = (css: string) => css.replace(/\/\*[\s\S]*?\*\//g, '')

const css = new Map(files.map((f) => [f, stripComments(read(f))]))
const all = [...css.values()].join('\n')

describe('styles contract', () => {
  it('defines every custom property that any stylesheet uses', () => {
    const defined = new Set([...all.matchAll(/(--[a-z0-9-]+)\s*:/gi)].map((m) => m[1]))
    const used = new Set([...all.matchAll(/var\(\s*(--[a-z0-9-]+)/gi)].map((m) => m[1]))
    const missing = [...used].filter((v) => !defined.has(v)).sort()
    expect(missing).toEqual([])
  })

  it('keeps color literals inside tokens.css', () => {
    const offenders = [...css.entries()]
      .filter(([name]) => name !== 'tokens.css')
      .filter(([, text]) => /#[0-9a-f]{3,8}\b/i.test(text))
      .map(([name]) => name)
    expect(offenders).toEqual([])
  })

  it('does not use !important', () => {
    const offenders = [...css.entries()]
      .filter(([, text]) => /!\s*important/i.test(text))
      .map(([name]) => name)
    expect(offenders).toEqual([])
  })

  it('serves the core tokens both themes rely on', () => {
    const tokens = css.get('tokens.css') ?? ''
    for (const name of ['--font-mono', '--radius-md', '--space-5', '--space-10']) {
      expect(tokens).toContain(`${name}:`)
    }
  })
})

// UI-5：文本与语义色必须满足 WCAG AA 正文对比度（≥4.5:1）。
const tokens = css.get('tokens.css') ?? ''
const darkAt = tokens.indexOf('html[data-theme="dark"]')
const themes: Record<string, Record<string, string>> = {
  light: parseTokens(tokens.slice(0, darkAt)),
  dark: parseTokens(tokens.slice(darkAt)),
}

function parseTokens(block: string): Record<string, string> {
  const map: Record<string, string> = {}
  for (const m of block.matchAll(/(--[a-z0-9-]+)\s*:\s*([^;]+);/gi)) {
    map[m[1]] = m[2].trim()
  }
  return map
}

function channel(value: number): number {
  const c = value / 255
  return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)
}

function luminance(hex: string): number {
  const n = hex.replace('#', '')
  return (
    0.2126 * channel(parseInt(n.slice(0, 2), 16)) +
    0.7152 * channel(parseInt(n.slice(2, 4), 16)) +
    0.0722 * channel(parseInt(n.slice(4, 6), 16))
  )
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

/** Resolve a hex or rgba() token to a solid hex over the given base color. */
function solid(value: string, base: string): string | null {
  if (/^#[0-9a-f]{6}$/i.test(value)) return value
  const m = value.match(/rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*(?:,\s*([\d.]+)\s*)?\)/)
  if (!m) return null
  const alpha = m[4] === undefined ? 1 : Number(m[4])
  const b = base.replace('#', '')
  const blend = (fg: string, bg: string) =>
    Math.round(Number(fg) * alpha + parseInt(bg, 16) * (1 - alpha))
      .toString(16)
      .padStart(2, '0')
  return `#${blend(m[1], b.slice(0, 2))}${blend(m[2], b.slice(2, 4))}${blend(m[3], b.slice(4, 6))}`
}

describe('contrast', () => {
  for (const [theme, map] of Object.entries(themes)) {
    for (const fg of ['--text-muted', '--text-faint']) {
      for (const bg of ['--bg', '--surface', '--surface-2', '--sidebar']) {
        it(`${theme}: ${fg} on ${bg}`, () => {
          expect(contrast(map[fg], map[bg])).toBeGreaterThanOrEqual(4.5)
        })
      }
    }
    it(`${theme}: success on success-soft`, () => {
      const bg = solid(map['--success-soft'], map['--surface'])
      expect(bg).not.toBeNull()
      expect(contrast(map['--success'], bg as string)).toBeGreaterThanOrEqual(4.5)
    })
    it(`${theme}: warning on warning-soft`, () => {
      const bg = solid(map['--warning-soft'], map['--surface'])
      expect(bg).not.toBeNull()
      expect(contrast(map['--warning'], bg as string)).toBeGreaterThanOrEqual(4.5)
    })
    it(`${theme}: danger on surface`, () => {
      expect(contrast(map['--danger'], map['--surface'])).toBeGreaterThanOrEqual(4.5)
    })
  }
})
