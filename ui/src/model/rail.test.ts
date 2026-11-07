import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { authoritySummary, healthRow, healthTone, sectionSummary } from './rail'

const kind = (kind: string, label: string, count: number) => ({ kind, label, count })
const counted = (label: string, kinds: ReturnType<typeof kind>[]) => ({ id: 'context' as const, label, kinds, items: [] })
const fixture = JSON.parse(readFileSync(new URL('../fixtures/overview.mock.json', import.meta.url), 'utf8'))

describe('section summary', () => {
  it('counts each type above zero, singular for one', () => {
    expect(sectionSummary(counted('Context', [kind('instructions', 'Instructions', 0), kind('rule', 'Rules', 1), kind('agent', 'Agents', 3)]))).toBe('Context: 1 rule, 3 agents')
    expect(sectionSummary(counted('Tools', [kind('mcp', 'MCP servers', 1)]))).toBe('Tools: 1 MCP server')
    expect(sectionSummary(counted('Context', [kind('instructions', 'Instructions', 2)]))).toBe('Context: 2 instructions files')
    expect(sectionSummary(counted('Context', [kind('review', 'Review instructions', 1)]))).toBe('Context: 1 review instructions file')
    expect(sectionSummary(counted('Settings', [kind('builtin', 'Built-ins', 1), kind('settings', 'Preferences', 4)]))).toBe('Settings: 1 built-in, 4 preferences')
  })

  it('says what is missing when nothing is counted', () => {
    expect(sectionSummary(counted('Lifecycle', [kind('hook', 'Hooks', 0)]))).toBe('Lifecycle: no hooks yet')
    expect(sectionSummary(counted('Context', [kind('rule', 'Rules', 0), kind('agent', 'Agents', 0)]))).toBe('Context: nothing set up yet')
  })
})

describe('authority summary', () => {
  it('counts the rules in every permission list', () => {
    const authority = fixture.sections.find((section: { id: string }) => section.id === 'authority')
    expect(sectionSummary(authority)).toBe('Authority: 11 allowed, 3 ask, 4 denied')
  })

  it('says so when no list holds a rule', () => {
    const value = (key: string, list: string[]) => ({ key, type: 'list', list })
    const item = { values: [value('permissions.allow', []), value('permissions.deny', [])] }
    expect(authoritySummary({ label: 'Authority', items: [item] as never })).toBe('Authority: no permissions yet')
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
    expect(healthRow({ errors: 1, warnings: 3 }, 12)).toEqual({ status: 'Needs attention', summary: 'Setup health: 4 to fix, 12 you could add' })
    expect(healthRow({ errors: 0, warnings: 0 }, 2)).toEqual({ status: 'All clear', summary: 'Setup health: nothing to fix, 2 you could add' })
    expect(healthRow({ errors: 0, warnings: 0 }, 0)).toEqual({ status: 'All clear', summary: 'Setup health: nothing to fix' })
  })
})
