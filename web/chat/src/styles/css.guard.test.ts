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
