import { describe, expect, it } from 'vitest'
import { cursorUrl } from './editor'

describe('editor file links', () => {
  it.each([
    '/tmp/rules with spaces.md',
    '/tmp/ågent#review?.yaml',
    '/C:/Users/Someone/project/#local?.md',
  ])('keeps the complete file path %s', (path) => {
    const url = new URL(cursorUrl(path))
    expect(url.protocol).toBe('cursor:')
    expect(url.host).toBe('file')
    expect(decodeURIComponent(url.pathname)).toBe(path)
    expect(url.search).toBe('')
    expect(url.hash).toBe('')
  })
})
