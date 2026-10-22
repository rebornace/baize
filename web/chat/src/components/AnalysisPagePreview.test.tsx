import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { AnalysisPagePreview } from './AnalysisPagePreview'

vi.mock('../gateContext', () => ({
  useGate: () => ({ gateEnabled: false }),
}))

describe('AnalysisPagePreview', () => {
  it('shows human open link and hides iframe until preview expanded', () => {
    const html = renderToStaticMarkup(
      createElement(AnalysisPagePreview, { artifactUrl: '/v0/artifacts/art_demo' }),
    )
    expect(html).toContain('打开分析页')
    expect(html).not.toContain('/v0/artifacts/art_demo')
    expect(html).not.toContain('<iframe')
    expect(html).toContain('展开预览')
  })
})
