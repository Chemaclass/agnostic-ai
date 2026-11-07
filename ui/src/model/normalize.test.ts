import { describe, expect, it } from 'vitest'
import { normalizeOverviewView } from './normalize'

describe('normalizeOverviewView repository', () => {
  it('keeps a GitHub repository page', () => {
    const view = normalizeOverviewView({ repository: { url: 'https://github.com/octo/hello', name: 'octo/hello' } })
    expect(view.repository).toEqual({ url: 'https://github.com/octo/hello', name: 'octo/hello' })
  })

  it('drops a link to another site or a malformed value', () => {
    for (const repository of [
      { url: 'https://evil.example', name: 'octo/hello' },
      { url: 'https://github.com.evil.example/octo/hello', name: 'octo/hello' },
      { url: 'javascript:alert(1)', name: 'octo/hello' },
      { url: 'https://github.com/octo/hello', name: {} },
      'https://github.com/octo/hello',
      null,
    ]) {
      expect(normalizeOverviewView({ repository }).repository).toBeUndefined()
    }
  })
})
