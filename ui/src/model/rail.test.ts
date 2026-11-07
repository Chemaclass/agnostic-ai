import { describe, expect, it } from 'vitest'
import { healthTone, sectionSummary } from './rail'

const kind = (kind: string, label: string, count: number) => ({ kind, label, count })

describe('section summary', () => {
  it('counts each type above zero, singular for one', () => {
    expect(sectionSummary({ label: 'Context', kinds: [kind('instructions', 'Instructions', 0), kind('rule', 'Rules', 1), kind('agent', 'Agents', 3)] })).toBe('Context: 1 rule, 3 agents')
    expect(sectionSummary({ label: 'Tools', kinds: [kind('mcp', 'MCP servers', 1)] })).toBe('Tools: 1 MCP server')
    expect(sectionSummary({ label: 'Context', kinds: [kind('instructions', 'Instructions', 2)] })).toBe('Context: 2 instructions files')
    expect(sectionSummary({ label: 'Settings', kinds: [kind('builtin', 'Built-ins', 1), kind('settings', 'Preferences', 4)] })).toBe('Settings: 1 built-in, 4 preferences')
  })

  it('says what is missing when nothing is counted', () => {
    expect(sectionSummary({ label: 'Lifecycle', kinds: [kind('hook', 'Hooks', 0)] })).toBe('Lifecycle: no hooks yet')
    expect(sectionSummary({ label: 'Context', kinds: [kind('rule', 'Rules', 0), kind('agent', 'Agents', 0)] })).toBe('Context: nothing set up yet')
  })
})

describe('health tone', () => {
  it('ranks errors over warnings', () => {
    expect(healthTone({ errors: 1, warnings: 3 })).toBe('error')
    expect(healthTone({ errors: 0, warnings: 3 })).toBe('warning')
    expect(healthTone({ errors: 0, warnings: 0 })).toBe('clean')
  })
})
