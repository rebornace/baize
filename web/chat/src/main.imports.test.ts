// web/chat/src/main.imports.test.ts
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const root = join(dirname(fileURLToPath(import.meta.url)))

describe('main CSS imports', () => {
  it('loads styles/chat.css and does not import ./style.css', () => {
    const main = readFileSync(join(root, 'main.tsx'), 'utf8')
    expect(main).toMatch(/import\s+['"]\.\/styles\/chat\.css['"]/)
    expect(main).not.toMatch(/import\s+['"]\.\/style\.css['"]/)
    expect(() => readFileSync(join(root, 'styles', 'chat.css'), 'utf8')).not.toThrow()
  })
})
