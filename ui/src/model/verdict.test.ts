import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import type { OverviewView, Section } from '../contract'
import { sectionSummary } from './rail'
import { emptyState, findingTallyText, ignoredFacts, sectionFacts, sectionFindings, sectionVerdict, supportSummary, tallyFindings } from './verdict'

const fixture: OverviewView = JSON.parse(readFileSync(new URL('../fixtures/overview.mock.json', import.meta.url), 'utf8'))
const section = (id: string) => fixture.sections.find((s) => s.id === id)!
const kind = (kind: string, label: string, count: number) => ({ kind, label, count })
const list = (key: string, count: number) => ({ key, type: 'list' as const, list: Array.from({ length: count }, (_, i) => `rule-${i}`) })
const item = (values: ReturnType<typeof list>[]) => ({ kind: 'settings', name: 'p', label: 'p', description: '', path: 'p', layer: 'project', values, findings: [] })
const empty = (id: Section['id'], kinds = [] as ReturnType<typeof kind>[]) => ({ id, kinds, items: [] })

describe('section verdict', () => {
  it('reads every section of the mock overview in plain words', () => {
    expect(sectionVerdict(section('context'))).toBe('Your setup configures 1 rule and 3 agents.')
    expect(sectionVerdict(section('authority'))).toBe('Your rules allow 11 actions, ask before 3, and block 4.')
    expect(sectionVerdict(section('tools'))).toBe('Your agents can use 3 MCP servers.')
    expect(sectionVerdict(section('lifecycle'))).toBe('Nothing runs around the agent yet.')
  })

  it('says what the setup configures when no set-up tool supports MCP servers', () => {
    const tools = (supported: string[], unsupported: string[]) => ({ ...section('tools'), support: [{ kind: 'mcp', label: 'MCP servers', supported, unsupported }] })
    expect(sectionVerdict(tools([], ['kiro']))).toBe('Your setup configures 3 MCP servers.')
    expect(sectionVerdict(tools([], []))).toBe('Your setup configures 3 MCP servers.')
    expect(sectionVerdict({ ...section('tools'), support: undefined })).toBe('Your setup configures 3 MCP servers.')
    expect(sectionVerdict(tools(['claude'], []))).toBe('Your agents can use 3 MCP servers.')
  })

  it('names the first count and skips empty permission lists', () => {
    expect(sectionVerdict({ ...empty('authority'), items: [item([list('permissions.ask', 1), list('permissions.deny', 2)])] })).toBe('Your rules ask before 1 action and block 2.')
    expect(sectionVerdict({ ...empty('authority'), items: [item([list('permissions.deny', 1)])] })).toBe('Your rules block 1 action.')
    expect(sectionVerdict(empty('authority'))).toBe('No allow, ask or block rules yet.')
  })

  it('says what is missing when a section is empty', () => {
    expect(sectionVerdict(empty('context', [kind('rule', 'Rules', 0)]))).toBe('Your setup configures no instructions yet.')
    expect(sectionVerdict(empty('tools', [kind('mcp', 'MCP servers', 0)]))).toBe('Your setup configures no extra tools yet.')
    expect(sectionVerdict(empty('settings'))).toBe('Your tools run on their own defaults.')
    expect(sectionVerdict(empty('context', [kind('instructions', 'Instructions', 1), kind('skill', 'Skills', 2)]))).toBe('Your setup configures 1 instructions file and 2 skills.')
  })

  it('counts hooks and the stages they run at', () => {
    const hook = (group: string) => ({ ...item([]), kind: 'hook', group })
    expect(sectionVerdict({ ...empty('lifecycle'), items: [hook('pre-tool'), hook('pre-tool'), hook('session-start')] })).toBe('3 hooks are set to run at 2 stages of a session.')
    expect(sectionVerdict({ ...empty('lifecycle'), items: [hook('pre-tool')] })).toBe('1 hook is set to run at 1 stage of a session.')
  })
})

describe('turned-off hooks', () => {
  const hook = (group: string, disabled = false) => ({ ...item([]), kind: 'hook', group, fields: disabled ? [{ key: 'disabled', type: 'bool' as const, value: 'true' }] : [] })
  const lifecycle = (hooks: ReturnType<typeof hook>[]) => ({ ...empty('lifecycle', [kind('hook', 'Hooks', hooks.length)]), items: hooks })

  it('counts every hook when all run', () => {
    const section = lifecycle([hook('pre-tool'), hook('session-start')])
    expect(sectionVerdict(section)).toBe('2 hooks are set to run at 2 stages of a session.')
    expect(sectionFacts(section)).toEqual([])
    expect(sectionSummary({ ...section, label: 'Lifecycle' })).toBe('Lifecycle: 2 hooks')
  })

  it('leaves turned-off hooks and their stages out of the verdict and names them in the facts', () => {
    const section = lifecycle([hook('pre-tool'), hook('session-start', true)])
    expect(sectionVerdict(section)).toBe('1 hook is set to run at 1 stage of a session.')
    expect(sectionFacts(section)).toEqual(['1 hook turned off'])
    expect(sectionSummary({ ...section, label: 'Lifecycle' })).toBe('Lifecycle: 1 hook')
  })

  it('says none are set to run when every hook is turned off', () => {
    const section = lifecycle([hook('pre-tool', true), hook('stop', true)])
    expect(sectionVerdict(section)).toBe('No hooks are set to run.')
    expect(sectionFacts(section)).toEqual(['2 hooks turned off'])
    const support = [{ kind: 'hook', label: 'Hooks', supported: ['claude'], unsupported: ['kiro'] }]
    expect(sectionFacts({ ...section, support }, (target) => (target === 'kiro' ? 'Kiro' : target))).toEqual(['2 hooks turned off'])
    expect(sectionSummary({ ...section, label: 'Lifecycle' })).toBe('Lifecycle: 2 hooks turned off')
  })
})

describe('default permission mode', () => {
  const mode = (value: string) => ({ key: 'permissions.default-mode', label: 'Default permission mode', description: '', set: [{ spec: 'settings', path: 'settings.yaml', values: [{ key: 'permissions.default-mode', type: 'text' as const, value }] }] })

  it('names a mode set without any rule lists', () => {
    const section = { ...empty('authority'), fields: [mode('bypassPermissions')] }
    expect(sectionVerdict(section)).toBe('No allow, ask or block rules yet.')
    expect(sectionFacts(section, undefined, true)).toEqual(['Default mode: bypassPermissions'])
  })

  it('adds the mode beside the rule counts', () => {
    const authority = { ...section('authority'), fields: [mode('acceptEdits')] }
    expect(sectionVerdict(authority)).toBe('Your rules allow 11 actions, ask before 3, and block 4.')
    expect(sectionFacts(authority, undefined, true)).toEqual(['Default mode: acceptEdits', '2 protected paths'])
  })

  it('leaves the mode out of a project view, where it applies only through global sync', () => {
    expect(sectionFacts({ ...empty('authority'), fields: [mode('bypassPermissions')] })).toEqual([])
  })

  it('prefers the mode a local file sets', () => {
    const field = mode('acceptEdits')
    field.set.push({ ...field.set[0], path: 'local/settings.yaml', local: true, values: [{ ...field.set[0].values[0], value: 'plan' }] } as typeof field.set[0])
    expect(sectionFacts({ ...empty('authority'), fields: [field] }, undefined, true)).toEqual(['Default mode: plan'])
  })

  it('says nothing about a mode when none is set', () => {
    expect(sectionVerdict(empty('authority'))).toBe('No allow, ask or block rules yet.')
    expect(sectionFacts(empty('authority'))).toEqual([])
    expect(sectionFacts(section('authority'))).toEqual(['2 protected paths'])
  })
})

describe('section facts', () => {
  const name = (target: string) => ({ claude: 'Claude Code', kiro: 'Kiro', codex: 'Codex' })[target] ?? target
  const permissions = (supported: string[], unsupported: string[]) => ({ ...section('authority'), support: [{ kind: 'settings', label: 'Permissions', supported, unsupported }] })

  it('says nothing about support when every tool applies the rules', () => {
    expect(ignoredFacts(permissions(['claude', 'codex'], []), name)).toEqual([])
  })

  it('names the tools that ignore a configured type', () => {
    expect(ignoredFacts(permissions(['claude'], ['kiro']), name)).toEqual(['Kiro ignores permissions'])
    expect(sectionFacts(permissions(['claude'], ['kiro', 'codex']), name)).toEqual(['2 protected paths', 'Kiro and Codex ignore permissions'])
    expect(ignoredFacts(permissions(['claude'], ['a', 'b', 'c', 'd']), name)).toEqual(['4 of 5 tools ignore permissions'])
  })

  it('skips types with nothing configured', () => {
    expect(ignoredFacts({ ...empty('context', [kind('skill', 'Skills', 0)]), support: [{ kind: 'skill', label: 'Skills', supported: [], unsupported: ['kiro'] }] }, name)).toEqual([])
  })

  it('counts review instructions as files', () => {
    expect(sectionVerdict(empty('context', [kind('review', 'Review instructions', 1)]))).toBe('Your setup configures 1 review instructions file.')
    expect(sectionVerdict(empty('context', [kind('review', 'Review instructions', 2)]))).toBe('Your setup configures 2 review instructions files.')
  })

  it('adds what the verdict leaves out', () => {
    expect(sectionFacts(section('authority'))).toEqual(['2 protected paths'])
    expect(sectionFacts(section('tools'))).toEqual(['1 more server found in your tools', '1 MCP server turned off'])
    expect(sectionFacts(section('settings'))).toEqual(['1 config file'])
    expect(sectionFacts(section('context'))).toEqual([])
  })

  it('counts findings on the section and its rows', () => {
    const tally = tallyFindings(sectionFindings(section('authority')))
    expect(tally).toEqual({ errors: 0, warnings: 1 })
    expect(findingTallyText(tally)).toBe('1 warning')
    expect(findingTallyText({ errors: 2, warnings: 1 })).toBe('2 errors, 1 warning')
    expect(findingTallyText({ errors: 0, warnings: 0 })).toBeUndefined()
  })
})

describe('support summary', () => {
  const support = (label: string, supported: string[], unsupported: string[]) => ({ kind: label.toLowerCase(), label, supported, unsupported })

  it('groups types by how many tools support them', () => {
    expect(supportSummary(section('context').support)).toBe('3 of 3 tools support rules, agents, and skills')
    expect(supportSummary([support('Rules', ['a', 'b'], ['c']), support('Ignore files', ['a'], ['b', 'c'])])).toBe('2 of 3 tools support rules; 1 of 3 supports ignore files')
    expect(supportSummary([support('Rules', [], [])])).toBe('No coding tool is set up yet')
    expect(supportSummary([])).toBe('')
  })
})

describe('empty state', () => {
  const name = (target: string) => ({ claude: 'Claude Code', codex: 'Codex' })[target] ?? target

  it('says what the type is and which tools use it', () => {
    expect(emptyState(kind('skill', 'Skills', 0), { kind: 'skill', label: 'Skills', supported: ['claude', 'codex'], unsupported: [] }, name)).toEqual({
      title: 'No skills yet.',
      purpose: 'A skill is a workflow the agent runs on request, such as cutting a release.',
      tools: 'Claude Code and Codex support skills.',
    })
    expect(emptyState(kind('instructions', 'Instructions', 0), undefined, name).title).toBe('No instructions files yet.')
    expect(emptyState(kind('skill', 'Skills', 0), { kind: 'skill', label: 'Skills', supported: [], unsupported: ['codex'] }, name).tools).toBe('None of your tools support skills yet.')
  })

  it('says only No matches. while a filter hides every row', () => {
    expect(emptyState(kind('skill', 'Skills', 0), undefined, name, true)).toEqual({ title: 'No matches.' })
  })
})
