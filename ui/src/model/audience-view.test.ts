import { describe, expect, it } from 'vitest'
import type { SettingField, Value } from '../contract'
import { builtinEnabled, comparisonValue, sourcesForAudience } from './audience-view'

type FieldSource = SettingField['set'][number]

describe('built-in audience', () => {
  it('shows a personal enable and the shared disable separately', () => {
    const item = { enabled: true, local: true }
    expect(builtinEnabled(item, 'shared')).toBe(false)
    expect(builtinEnabled(item, 'personal')).toBe(true)
  })

  it('shows a personal disable and the shared enable separately', () => {
    const item = { enabled: false, local: true }
    expect(builtinEnabled(item, 'shared')).toBe(true)
    expect(builtinEnabled(item, 'personal')).toBe(false)
  })

  it('keeps matching and inherited states in both views', () => {
    const item = { enabled: true, local: false }
    expect(builtinEnabled(item, 'shared')).toBe(true)
    expect(builtinEnabled(item, 'personal')).toBe(true)
  })
})

describe('override comparisons', () => {
  const cases: { name: string; values: Value[]; key: string; expected?: string }[] = [
    { name: 'enabled', values: [{ key: 'x-tool.verbose', type: 'bool', value: 'true' }], key: 'x-tool.verbose', expected: 'On' },
    { name: 'disabled', values: [{ key: 'x-tool.verbose', type: 'bool', value: 'false' }], key: 'x-tool.verbose', expected: 'Off' },
    { name: 'cleared group', values: [{ key: 'x-tool', type: 'text', value: 'null' }], key: 'x-tool.verbose', expected: 'Cleared' },
    { name: 'configured group', values: [{ key: 'x-tool.verbose', type: 'bool', value: 'true' }], key: 'x-tool', expected: 'Configured' },
    { name: 'empty list', values: [{ key: 'x-tool.paths', type: 'list', list: [] }], key: 'x-tool.paths', expected: 'Empty' },
    { name: 'list', values: [{ key: 'x-tool.paths', type: 'list', list: ['src', 'tests'] }], key: 'x-tool.paths', expected: 'src, tests' },
    { name: 'unset', values: [], key: 'model' },
  ]
  for (const { name, values, key, expected } of cases) {
    it(`describes ${name} without confusing it with a different override`, () => {
      expect(comparisonValue(values, key)).toBe(expected)
    })
  }
})

describe('settings audience', () => {
  const shared: FieldSource = { spec: 'defaults', path: 'settings/defaults.yaml', values: [
    { key: 'x-claude.verbose', type: 'bool', value: 'false' },
    { key: 'x-claude.theme', type: 'text', value: 'light' },
  ] }
  const personal: FieldSource = { spec: 'defaults', path: 'local/settings/defaults.yaml', local: true, values: [
    { key: 'x-claude.verbose', type: 'bool', value: 'true' },
  ] }

  it('shows the shared value when a personal source overrides it', () => {
    expect(sourcesForAudience([personal, shared], 'shared')).toEqual([shared])
  })

  it('shows personal overrides and the shared values they inherit', () => {
    const sources = sourcesForAudience([personal, shared], 'personal')
    expect(sources.flatMap((source) => source.values)).toEqual([personal.values[0], shared.values[1]])
    expect(sources[1].path).toBe(shared.path)
    expect(shared.values).toHaveLength(2)
  })

  it('keeps values from independent specs visible', () => {
    const other = { ...shared, spec: 'another', path: 'settings/another.yaml' }
    expect(sourcesForAudience([personal, other], 'personal')).toEqual([personal, other])
  })

  it('keeps a personal null instead of inheriting a cleared shared key', () => {
    const cleared: FieldSource = { ...personal, values: [{ key: 'x-claude.verbose', type: 'text', value: 'null' }] }
    expect(sourcesForAudience([cleared, shared], 'personal').flatMap((source) => source.values)).toEqual([cleared.values[0], shared.values[1]])
  })
})
