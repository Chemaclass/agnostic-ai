import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { authoritySummary, healthRow, healthTone, sectionSummary } from './rail'

const kind = (kind: string, label: string, count: number) => ({ kind, label, count })
const counted = (label: string, kinds: ReturnType<typeof kind>[]) => ({ id: 'context' as const, label, kinds, items: [] })
const fixture = JSON.parse(readFileSync(new URL('../fixtures/overview.mock.json', import.meta.url), 'utf8'))

describe('section summary', () => {
  it('counts each type above zero, singular for one', () => {
    expect(sectionSummary(counted('Context', [kind('instructions', 'Instructions', 0), kind('rule', 'Rules', 1), kind('agent', 'Agents', 3)]))).toBe('1 rule, 3 agents')
    expect(sectionSummary(counted('Tools', [kind('mcp', 'MCP servers', 1)]))).toBe('1 MCP server')
    expect(sectionSummary(counted('Context', [kind('instructions', 'Instructions', 2)]))).toBe('2 instructions files')
    expect(sectionSummary(counted('Context', [kind('review', 'Review instructions', 1)]))).toBe('1 review instructions file')
  })

  it('counts the built-ins on, else the preferences set, on Settings', () => {
    const settings = (items: { kind: string; enabled?: boolean }[], preferences: number) => ({ id: 'settings' as const, kinds: [kind('builtin', 'Built-ins', items.length), kind('settings', 'Preferences', preferences)], items: items as never })
    expect(sectionSummary(settings([{ kind: 'builtin', enabled: true }, { kind: 'builtin', enabled: true }, { kind: 'builtin', enabled: false }], 1))).toBe('2 built-ins on')
    expect(sectionSummary(settings([{ kind: 'builtin', enabled: false }], 1))).toBe('1 preference set')
    expect(sectionSummary(settings([], 0))).toBe('Nothing set up yet')
  })

  it('says what is missing when nothing is counted', () => {
    expect(sectionSummary(counted('Lifecycle', [kind('hook', 'Hooks', 0)]))).toBe('No hooks yet')
    expect(sectionSummary(counted('Context', [kind('rule', 'Rules', 0), kind('agent', 'Agents', 0)]))).toBe('Nothing set up yet')
  })
})

describe('authority summary', () => {
  it('counts the rules in every permission list', () => {
    const authority = fixture.sections.find((section: { id: string }) => section.id === 'authority')
    expect(sectionSummary(authority)).toBe('4 blocked, 3 ask, 11 allowed')
  })

  it('says so when no list holds a rule', () => {
    const value = (key: string, list: string[]) => ({ key, type: 'list', list })
    const item = { values: [value('permissions.allow', []), value('permissions.deny', [])] }
    expect(authoritySummary({ items: [item] as never })).toBe('No rules yet')
  })
})

describe('health tone', () => {
  it('ranks errors over warnings', () => {
    expect(healthTone({ errors: 1, warnings: 3 })).toBe('error')
    expect(healthTone({ errors: 0, warnings: 3 })).toBe('warning')
    expect(healthTone({ errors: 0, warnings: 0 })).toBe('clean')
  })
})

describe('health row', () => {
  it('counts findings to fix and features to add', () => {
    expect(healthRow({ errors: 1, warnings: 3 }, 12)).toEqual({ status: 'Needs attention', summary: '4 to fix, 12 you could add' })
    expect(healthRow({ errors: 0, warnings: 0 }, 2)).toEqual({ status: 'All clear', summary: 'Nothing to fix, 2 you could add' })
    expect(healthRow({ errors: 0, warnings: 0 }, 0)).toEqual({ status: 'All clear', summary: 'Nothing to fix' })
  })
})
