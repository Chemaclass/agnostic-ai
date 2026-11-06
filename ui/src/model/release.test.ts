import { describe, expect, it } from 'vitest'
import { requiredRelease, servingRelease } from './release'

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

describe('required release', () => {
  it('stays away when the config sets no requires', () => {
    expect(requiredRelease({ global: false, version: '0.80.0' })).toBeUndefined()
  })

  it('warns when the serving release is outside requires', () => {
    expect(requiredRelease({ global: false, version: '0.80.0', requires: '>=0.81.0', requiresMet: false })).toEqual({
      label: 'requires >=0.81.0',
      state: 'unmet',
      title: 'This project needs agnostic-ai >=0.81.0, and v0.80.0 is outside it. Run agnostic-ai sync to see the fix.',
    })
  })

  it('shows requires quietly when the serving release meets it', () => {
    expect(requiredRelease({ global: true, version: '0.81.0', requires: '>=0.81.0', requiresMet: true })).toEqual({
      label: 'requires >=0.81.0',
      state: 'met',
      title: 'The global setup needs agnostic-ai >=0.81.0; v0.81.0 meets it.',
    })
  })

  it('says a checkout build is not checked', () => {
    expect(requiredRelease({ global: false, version: 'main@b434c92e1a2b', requires: '>=0.81.0', requiresMet: null })).toMatchObject({ state: 'unchecked' })
  })
})
