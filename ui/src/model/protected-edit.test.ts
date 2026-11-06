import { describe, expect, it } from 'vitest'
import { hidesValue, parseProtectedPreview, protectedSelection } from './protected-edit'
import type { SettingField } from '../contract'

describe('protected paths editing', () => {
  const project = '.agnostic-ai/settings/permissions.yaml'
  const local = '.agnostic-ai/local/settings/permissions.yaml'
  const field: SettingField = {
    key: 'protected', label: 'Protected paths', description: '',
    set: [{ spec: 'permissions', path: project, values: [
      { key: 'protected.paths', type: 'list', list: ['.github/**', 'composer.lock'] },
      { key: 'protected.reason', type: 'text', value: 'CI only.' },
    ] }],
    editable: { kind: 'protected-list', path: project, localPath: local },
  }

  it('reads the block each file writes and leaves an absent decision empty', () => {
    expect(protectedSelection(field, [], project)).toEqual({
      shared: { paths: ['.github/**', 'composer.lock'], decision: '', reason: 'CI only.' },
      personal: null, merged: { paths: ['.github/**', 'composer.lock'], decision: '', reason: 'CI only.' }, personalSet: false, personalPath: local,
    })
  })

  it('reads a local block separately from the project block', () => {
    const withLocal = { ...field, set: [...field.set, { spec: 'permissions', path: local, local: true, values: [{ key: 'protected.paths', type: 'list' as const, list: ['notes/'] }, { key: 'protected.decision', type: 'text' as const, value: 'deny' }] }] }
    expect(protectedSelection(withLocal, [], project)).toMatchObject({ personal: { paths: ['notes/'], decision: 'deny', reason: '' }, personalSet: true, personalPath: local })
  })

  it('refuses a preview without the protected block shape', () => {
    const preview = { schemaVersion: 'protected-edit/v1', file: project, audience: 'shared', before: null, protected: { paths: ['a'], decision: 'ask', reason: '' }, diff: '', expected: 'x', syncCommand: 'agnostic-ai sync' }
    expect(parseProtectedPreview(preview)).toEqual(preview)
    expect(() => parseProtectedPreview({ ...preview, protected: { paths: 'a' } })).toThrow('invalid protected paths preview')
  })

  it('reads an empty written reason as empty and spots hidden values', () => {
    const empty = { ...field, set: [{ ...field.set[0], values: [...field.set[0].values.filter((value) => value.key !== 'protected.reason'), { key: 'protected.reason', type: 'text' as const, value: '""' }] }] }
    const block = protectedSelection(empty, [], project).shared
    expect(block?.reason).toBe('')
    expect(hidesValue(block)).toBe(false)
    expect(hidesValue({ paths: ['<redacted>'], decision: '', reason: '' })).toBe(true)
  })

  it('fills a local block that leaves keys out from the project block, so a project deny stays', () => {
    const deny = { ...field, set: [
      { ...field.set[0], values: [...field.set[0].values, { key: 'protected.decision', type: 'text' as const, value: 'deny' }] },
      { spec: 'permissions', path: local, local: true, values: [{ key: 'protected.paths', type: 'list' as const, list: ['notes/'] }] },
    ] }
    const selection = protectedSelection(deny, [], project)
    expect(selection.personal).toEqual({ paths: ['notes/'], decision: '', reason: '' })
    expect(selection.merged).toEqual({ paths: ['notes/'], decision: 'deny', reason: 'CI only.' })
  })

  it('keeps the project paths and reason a local block leaves out', () => {
    const decisionOnly = { ...field, set: [...field.set, { spec: 'permissions', path: local, local: true, values: [{ key: 'protected.decision', type: 'text' as const, value: 'deny' }] }] }
    expect(protectedSelection(decisionOnly, [], project).merged).toEqual({ paths: ['.github/**', 'composer.lock'], decision: 'deny', reason: 'CI only.' })
  })
})
