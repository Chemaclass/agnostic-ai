import { describe, expect, it } from 'vitest'
import type { SettingField } from '../contract'
import { hasRules, modeSummary, protectedSummary, ruleRows } from './authority'

const shared = { spec: 'permissions', path: 'shared.yaml' }
const local = { spec: 'permissions', path: 'local.yaml', local: true }
const rules = (key: string, list: string[]) => [{ key, type: 'list' as const, list }]
const field = (key: string, set: SettingField['set']): SettingField => ({ key, label: key, description: '', set })

describe('rule rows', () => {
  const ask = field('permissions.ask', [{ ...shared, values: rules('permissions.ask', ['a', 'b']) }, { ...local, values: rules('permissions.ask', ['c']) }])

  it('lists the project rules in the project view, with their place in the file', () => {
    expect(ruleRows(ask, 'shared')).toEqual([{ rule: 'a', index: 0, path: 'shared.yaml', local: false }, { rule: 'b', index: 1, path: 'shared.yaml', local: false }])
  })

  it('lists the local list in place of the project one in the local view', () => {
    expect(ruleRows(ask, 'personal')).toEqual([{ rule: 'c', index: 0, path: 'local.yaml', local: true }])
  })

  it('shows nothing for a list a local file clears', () => {
    const cleared = field('permissions.ask', [{ ...shared, values: rules('permissions.ask', ['a']) }, { ...local, values: [{ key: 'permissions.ask', type: 'text', value: 'null' }] }])
    expect(ruleRows(cleared, 'personal')).toEqual([])
    expect(hasRules([cleared], 'personal')).toBe(false)
    expect(hasRules([cleared], 'shared')).toBe(true)
  })
})

describe('also on this page', () => {
  it('sums up protected paths and their decision', () => {
    const paths = field('protected', [{ ...shared, values: [{ key: 'protected.paths', type: 'list', list: ['.github/**', 'migrations/**'] }, { key: 'protected.decision', type: 'text', value: 'ask' }] }])
    expect(protectedSummary(paths, 'shared')).toBe('2 paths, ask first')
    expect(protectedSummary(field('protected', []), 'shared')).toBe('None')
    const off = field('protected', [...paths.set, { ...local, values: [{ key: 'protected', type: 'text', value: 'null' }] }])
    expect(protectedSummary(off, 'personal')).toBe('Off for you')
  })

  it('names the default mode, preferring a local one', () => {
    const mode = field('permissions.default-mode', [{ ...shared, values: [{ key: 'permissions.default-mode', type: 'text', value: 'plan' }] }, { ...local, values: [{ key: 'permissions.default-mode', type: 'text', value: 'acceptEdits' }] }])
    expect(modeSummary(mode, 'shared')).toBe('plan')
    expect(modeSummary(mode, 'personal')).toBe('acceptEdits')
    expect(modeSummary(field('permissions.default-mode', []), 'shared')).toBe('Not set')
  })
})
