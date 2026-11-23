import { describe, expect, it } from 'vitest'
import { inferMediaType, isImageAttachment } from './api'

function fakeFile(name: string, type: string): File {
  return new File(['x'], name, { type })
}

describe('inferMediaType', () => {
  it('maps .md to text/markdown even when the browser reports an empty type', () => {
    // Windows commonly gives .md files an empty File.type.
    expect(inferMediaType(fakeFile('notes.md', ''))).toBe('text/markdown')
  })

  it('normalizes .csv away from application/vnd.ms-excel to text/csv', () => {
    // Windows often reports .csv as application/vnd.ms-excel, which the
    // backend would reject as unsupported_attachment.
    expect(inferMediaType(fakeFile('pets.csv', 'application/vnd.ms-excel'))).toBe(
      'text/csv',
    )
  })

  it('maps .txt to text/plain', () => {
    expect(inferMediaType(fakeFile('a.txt', ''))).toBe('text/plain')
  })

  it('maps .docx and .xlsx to the OOXML MIMEs the backend accepts', () => {
    expect(inferMediaType(fakeFile('a.docx', ''))).toBe(
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    )
    expect(inferMediaType(fakeFile('a.xlsx', 'application/vnd.ms-excel'))).toBe(
      'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    )
  })

  it('maps .pdf to application/pdf', () => {
    expect(inferMediaType(fakeFile('a.pdf', ''))).toBe('application/pdf')
  })

  it('maps image extensions to the canonical image MIMEs', () => {
    expect(inferMediaType(fakeFile('a.png', ''))).toBe('image/png')
    expect(inferMediaType(fakeFile('a.jpg', 'image/jpeg'))).toBe('image/jpeg')
    expect(inferMediaType(fakeFile('a.jpeg', ''))).toBe('image/jpeg')
    expect(inferMediaType(fakeFile('a.webp', ''))).toBe('image/webp')
    expect(inferMediaType(fakeFile('a.gif', ''))).toBe('image/gif')
  })

  it('is case-insensitive on the extension', () => {
    expect(inferMediaType(fakeFile('NOTES.MD', ''))).toBe('text/markdown')
    expect(inferMediaType(fakeFile('Pic.PNG', ''))).toBe('image/png')
  })

  it('falls back to the browser type for unknown extensions', () => {
    expect(inferMediaType(fakeFile('a.xyz', 'application/x-foo'))).toBe(
      'application/x-foo',
    )
  })

  it('falls back to application/octet-stream when nothing is known', () => {
    expect(inferMediaType(fakeFile('noext', ''))).toBe('application/octet-stream')
  })

  it('produces media_types that isImageAttachment recognizes as images', () => {
    // Sanity: the inferred image MIMEs must flow through isImageAttachment.
    for (const name of ['a.png', 'a.jpg', 'a.jpeg', 'a.webp', 'a.gif']) {
      expect(isImageAttachment(inferMediaType(fakeFile(name, '')))).toBe(true)
    }
    // Non-image inferences must not be flagged as images.
    expect(isImageAttachment(inferMediaType(fakeFile('a.md', '')))).toBe(false)
    expect(isImageAttachment(inferMediaType(fakeFile('a.csv', '')))).toBe(false)
  })
})
