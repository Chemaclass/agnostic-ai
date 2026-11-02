import { describe, expect, it } from 'vitest'
import { servingRelease } from './release'

describe('serving release', () => {
  it('names a published release and links its notes', () => {
    expect(servingRelease('0.91.0')).toEqual({ kind: 'release', label: 'v0.91.0', notesUrl: 'https://github.com/Chemaclass/agnostic-ai/releases/tag/v0.91.0' })
    expect(servingRelease('v0.92.0-rc.1')).toEqual({ kind: 'release', label: 'v0.92.0-rc.1', notesUrl: 'https://github.com/Chemaclass/agnostic-ai/releases/tag/v0.92.0-rc.1' })
  })

  it('shows a checkout build as it is, with no release to link', () => {
    expect(servingRelease('main@b434c92e1a2b')).toEqual({ kind: 'build', label: 'main@b434c92e1a2b' })
    expect(servingRelease('(devel)')).toEqual({ kind: 'build', label: '(devel)' })
  })
})
