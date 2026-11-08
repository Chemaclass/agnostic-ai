import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import type { OverviewView, Section } from '../contract'
import { sectionSummary } from './rail'
import { emptyState, findingTallyText, sectionFacts, sectionFindings, sectionVerdict, supportSummary, tallyFindings, verdictKey } from './verdict'

const fixture: OverviewView = JSON.parse(readFileSync(new URL('../fixtures/overview.mock.json', import.meta.url), 'utf8'))
const section = (id: string) => fixture.sections.find((s) => s.id === id)!
const kind = (kind: string, label: string, count: number) => ({ kind, label, count })
const list = (key: string, count: number) => ({ key, type: 'list' as const, list: Array.from({ length: count }, (_, i) => `rule-${i}`) })
const item = (values: ReturnType<typeof list>[]) => ({ kind: 'settings', name: 'p', label: 'p', description: '', path: 'p', layer: 'project', values, findings: [] })
const empty = (id: Section['id'], kinds = [] as ReturnType<typeof kind>[]) => ({ id, kinds, items: [] })

describe('section verdict', () => {
  it('reads every section of the mock overview in plain words', () => {
    expect(sectionVerdict(section('context'))).toBe('Your setup configures 1 rule and 3 agents.')
    expect(sectionVerdict(section('authority'))).toBe('4 actions are blocked and agents ask before 3.')
    expect(sectionVerdict(section('tools'))).toBe('Agents can use 3 MCP servers.')
    expect(sectionVerdict(section('lifecycle'))).toBe('Nothing runs around the agent yet.')
  })

  it('says what the setup configures when no set-up tool supports MCP servers', () => {
    const tools = (supported: string[], unsupported: string[]) => ({ ...section('tools'), support: [{ kind: 'mcp', label: 'MCP servers', supported, unsupported }] })
    expect(sectionVerdict(tools([], ['kiro']))).toBe('Your setup configures 3 MCP servers.')
    expect(sectionVerdict(tools([], []))).toBe('Your setup configures 3 MCP servers.')
    expect(sectionVerdict({ ...section('tools'), support: undefined })).toBe('Your setup configures 3 MCP servers.')
    expect(sectionVerdict(tools(['claude'], []))).toBe('Agents can use 3 MCP servers.')
  })

  it('leads with blocked rules, then ask, then allowed', () => {
    expect(sectionVerdict({ ...empty('authority'), items: [item([list('permissions.ask', 1), list('permissions.deny', 2)])] })).toBe('2 actions are blocked and agents ask before 1.')
    expect(sectionVerdict({ ...empty('authority'), items: [item([list('permissions.deny', 1)])] })).toBe('1 action is blocked.')
    expect(sectionVerdict({ ...empty('authority'), items: [item([list('permissions.ask', 2), list('permissions.allow', 3)])] })).toBe('Agents ask before 2 actions.')
    expect(sectionVerdict({ ...empty('authority'), items: [item([list('permissions.allow', 1)])] })).toBe('Agents run 1 action without asking.')
    expect(sectionVerdict(empty('authority'))).toBe('Each tool decides when to ask.')
  })

  it('says what is missing when a section is empty', () => {
    expect(sectionVerdict(empty('context', [kind('rule', 'Rules', 0)]))).toBe('Your setup configures no instructions yet.')
    expect(sectionVerdict(empty('tools', [kind('mcp', 'MCP servers', 0)]))).toBe('Your setup configures no extra tools yet.')
    expect(sectionVerdict(empty('settings'))).toBe('Your tools run on their own defaults.')
    expect(sectionVerdict(empty('context', [kind('instructions', 'Instructions', 1), kind('skill', 'Skills', 2)]))).toBe('Your setup configures 1 instructions file and 2 skills.')
  })

  it('leads Settings with the built-ins that are on, then the preferences set', () => {
    const builtin = (name: string, enabled: boolean) => ({ ...item([]), kind: 'builtin', name, enabled })
    const field = { key: 'model', label: 'Model', description: '', set: [{ spec: 'settings', path: 'settings.yaml', values: [{ key: 'model', type: 'text' as const, value: 'opus' }] }] }
    const settings = { ...empty('settings'), items: [builtin('handoff', true), builtin('memory', true), builtin('rtk', false)], fields: [field] }
    expect(sectionVerdict(settings)).toBe('2 built-ins are on.')
    expect(verdictKey(settings)).toBe('2 built-ins')
    expect(sectionFacts(settings, { on: 31, total: 32 })).toEqual(['1 preference set', '31 of 32 tools on'])
    const preferences = { ...empty('settings'), fields: [field] }
    expect(sectionVerdict(preferences)).toBe('1 preference is set.')
    expect(verdictKey(preferences)).toBe('1 preference')
    expect(sectionFacts(preferences)).toEqual([])
  })

  it('names the tools that hold servers agnostic-ai does not share yet', () => {
    const holder = (label: string) => ({ target: label.toLowerCase(), label, path: '~/.x', scope: 'user' as const })
    const tools = { ...section('tools'), native: [{ name: 'paper', holders: [holder('Claude Code'), holder('Codex')] }] }
    expect(sectionFacts(tools)[0]).toBe('1 more server found in Claude Code and Codex')
    expect(verdictKey(section('tools'))).toBe('3 MCP servers')
    const many = { ...tools, native: [{ name: 'paper', holders: ['A', 'B', 'C', 'D'].map(holder) }] }
    expect(sectionFacts(many)[0]).toBe('1 more server found in 4 of your tools')
  })

  it('counts hooks and the stages they run at', () => {
    const hook = (group: string) => ({ ...item([]), kind: 'hook', group })
    expect(sectionVerdict({ ...empty('lifecycle'), items: [hook('before-tool'), hook('before-tool'), hook('session-start')] })).toBe('3 hooks run at 2 stages.')
    expect(sectionVerdict({ ...empty('lifecycle'), items: [hook('session-start'), hook('session-start')] })).toBe('2 hooks run when a session starts.')
    expect(sectionVerdict({ ...empty('lifecycle'), items: [hook('before-tool')] })).toBe('1 hook runs before a tool runs.')
    const native = { ...empty('lifecycle'), stages: [{ id: 'native:TaskStart', label: 'TaskStart', description: '', targets: [] }], items: [hook('native:TaskStart')] }
    expect(sectionVerdict(native)).toBe('1 hook runs at TaskStart.')
  })

  it('counts the stages no live hook uses in the facts', () => {
    const hook = (group: string) => ({ ...item([]), kind: 'hook', group })
    const stage = (id: string) => ({ id, label: id, description: '', targets: [] })
    const stages = [stage('session-start'), stage('stop'), stage('session-end')]
    expect(sectionFacts({ ...empty('lifecycle'), stages, items: [hook('session-start')] })).toEqual(['2 stages have no hook'])
    expect(sectionFacts({ ...empty('lifecycle'), stages: stages.slice(0, 2), items: [hook('session-start')] })).toEqual(['1 stage has no hook'])
    expect(sectionFacts({ ...empty('lifecycle'), stages: stages.slice(0, 1), items: [hook('session-start')] })).toEqual([])
  })
})

describe('turned-off hooks', () => {
  const hook = (group: string, disabled = false) => ({ ...item([]), kind: 'hook', group, fields: disabled ? [{ key: 'disabled', type: 'bool' as const, value: 'true' }] : [] })
  const lifecycle = (hooks: ReturnType<typeof hook>[]) => ({ ...empty('lifecycle', [kind('hook', 'Hooks', hooks.length)]), items: hooks })

  it('counts every hook when all run', () => {
    const section = lifecycle([hook('before-tool'), hook('session-start')])
    expect(sectionVerdict(section)).toBe('2 hooks run at 2 stages.')
    expect(sectionFacts(section)).toEqual([])
    expect(sectionSummary(section)).toBe('2 hooks')
  })

  it('leaves turned-off hooks and their stages out of the verdict and names them in the facts', () => {
    const section = lifecycle([hook('before-tool'), hook('session-start', true)])
    expect(sectionVerdict(section)).toBe('1 hook runs before a tool runs.')
    expect(sectionFacts(section)).toEqual(['1 hook turned off'])
    expect(sectionSummary(section)).toBe('1 hook')
  })

  it('says none are set to run when every hook is turned off', () => {
    const section = lifecycle([hook('pre-tool', true), hook('stop', true)])
    expect(sectionVerdict(section)).toBe('No hooks are set to run.')
    expect(sectionFacts(section)).toEqual(['2 hooks turned off'])
    const support = [{ kind: 'hook', label: 'Hooks', supported: ['claude'], unsupported: ['kiro'] }]
    expect(sectionFacts({ ...section, support })).toEqual(['2 hooks turned off'])
    expect(sectionSummary(section)).toBe('2 hooks turned off')
  })
})

describe('authority facts', () => {
  it('says no rules exist yet', () => {
    expect(sectionFacts(empty('authority'))).toEqual(['No allow, ask or block rules yet'])
  })

  it('gives the allowed count the verdict leaves out', () => {
    expect(sectionFacts(section('authority'))).toEqual(['11 run without asking'])
    expect(sectionFacts({ ...empty('authority'), items: [item([list('permissions.deny', 1), list('permissions.allow', 1)])] })).toEqual(['1 runs without asking'])
  })

  it('adds nothing when the verdict already counts every rule', () => {
    expect(sectionFacts({ ...empty('authority'), items: [item([list('permissions.allow', 3)])] })).toEqual([])
    expect(sectionFacts({ ...empty('authority'), items: [item([list('permissions.deny', 3)])] })).toEqual([])
  })
})

describe('section facts', () => {
  const permissions = (supported: string[], unsupported: string[]) => ({ ...section('authority'), support: [{ kind: 'settings', label: 'Permissions', supported, unsupported }] })

  it('leaves tool support to the Tool support link', () => {
    expect(sectionFacts(permissions(['claude'], ['kiro', 'codex']))).toEqual(['11 run without asking'])
  })

  it('counts review instructions as files', () => {
    expect(sectionVerdict(empty('context', [kind('review', 'Review instructions', 1)]))).toBe('Your setup configures 1 review instructions file.')
    expect(sectionVerdict(empty('context', [kind('review', 'Review instructions', 2)]))).toBe('Your setup configures 2 review instructions files.')
  })

  it('adds what the verdict leaves out', () => {
    expect(sectionFacts(section('authority'))).toEqual(['11 run without asking'])
    expect(sectionFacts(section('tools'))).toEqual(['1 more server found in Claude Code', '1 MCP server turned off'])
    expect(sectionFacts(section('settings'))).toEqual([])
    expect(sectionFacts(section('settings'), { on: 31, total: 32 })).toEqual(['31 of 32 tools on'])
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
