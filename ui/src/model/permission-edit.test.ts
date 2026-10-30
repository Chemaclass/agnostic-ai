import { describe, expect, it } from 'vitest'
import { parsePermissionPreview, permissionSelection, rulesFromText } from './permission-edit'
import type { SettingField } from '../contract'

describe('permission editing', () => {
  const shared = '.agnostic-ai/settings/defaults.yaml'
  const personal = '.agnostic-ai/local/settings/defaults.yaml'
  const field: SettingField = {
    key: 'permissions.ask', label: 'Ask first', description: '',
    set: [
      { spec: 'defaults', path: shared, values: [{ key: 'permissions.ask', type: 'list', list: ['shell(git push *)'] }] },
      { spec: 'defaults', path: personal, local: true, values: [{ key: 'permissions.ask', type: 'list', list: [] }] },
      { spec: 'other', path: 'other.yaml', local: true, values: [{ key: 'permissions.ask', type: 'list', list: ['read'] }] },
    ],
    editable: { kind: 'permission-list', path: shared, localPath: personal },
  }

  it('keeps an empty personal list distinct from inheriting shared rules', () => {
    expect(permissionSelection(field, [], shared)).toEqual({ shared: ['shell(git push *)'], personal: [], personalSet: true, personalPath: personal })
    expect(permissionSelection({ ...field, set: field.set.filter((source) => source.path !== personal) }, [], shared)).toEqual({ shared: ['shell(git push *)'], personal: null, personalSet: false, personalPath: personal })
  })

  it('recognizes an explicit personal clear as an override that can be reset', () => {
    const cleared = { ...field, set: field.set.map((source) => source.path === personal ? { ...source, values: [{ key: field.key, type: 'text' as const, value: 'null' }] } : source) }
    expect(permissionSelection(cleared, [], shared)).toMatchObject({ personal: null, personalSet: true, personalPath: personal })
  })

  it('reads one rule per line while keeping the rule contents', () => {
    expect(rulesFromText('  shell(git status)\r\n\nread(src/**)\n')).toEqual(['shell(git status)', 'read(src/**)'])
  })

  it('associates a personal-only list with the chosen file by declared spec name', () => {
    const selected: SettingField = {
      ...field,
      set: field.set.filter((source) => source.path === personal),
      editable: { kind: 'permission-list', paths: [shared, 'other.yaml'], sources: [{ path: shared, spec: 'defaults', localPath: personal }, { path: 'other.yaml', spec: 'other', localPath: 'local/other.yaml' }] },
    }
    expect(permissionSelection(selected, [], shared)).toEqual({ shared: null, personal: [], personalSet: true, personalPath: personal })
    expect(permissionSelection(selected, [], 'other.yaml')).toEqual({ shared: null, personal: null, personalSet: false, personalPath: 'local/other.yaml' })
  })

  const preview = {
    schemaVersion: 'permission-edit/v1', file: shared, key: 'permissions.ask', audience: 'shared',
    before: null, rules: ['shell(git push *)'], diff: '+ ask:', expected: 'file-token', syncCommand: 'agnostic-ai sync',
  }

  it('keeps the exact scope, lists, and file diff the server will save', () => {
    expect(parsePermissionPreview(preview)).toEqual(preview)
  })

  it.each([
    { schemaVersion: 'permission-edit/v2' }, { audience: 'team' }, { key: 'permissions.default-mode' },
    { file: '' }, { expected: '' }, { before: [42] }, { rules: undefined }, { diff: false }, { syncCommand: '' },
  ])('refuses an invalid preview %j', (change) => {
    expect(() => parsePermissionPreview({ ...preview, ...change })).toThrow('invalid permission preview')
  })
})
