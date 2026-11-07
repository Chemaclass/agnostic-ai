import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import type { OverviewView, Section } from '../contract'
import { emptyState, findingTallyText, sectionFacts, sectionFindings, sectionVerdict, supportSummary, tallyFindings } from './verdict'

const fixture: OverviewView = JSON.parse(readFileSync(new URL('../fixtures/overview.mock.json', import.meta.url), 'utf8'))
const section = (id: string) => fixture.sections.find((s) => s.id === id)!
const kind = (kind: string, label: string, count: number) => ({ kind, label, count })
const list = (key: string, count: number) => ({ key, type: 'list' as const, list: Array.from({ length: count }, (_, i) => `rule-${i}`) })
const item = (values: ReturnType<typeof list>[]) => ({ kind: 'settings', name: 'p', label: 'p', description: '', path: 'p', layer: 'project', values, findings: [] })
const empty = (id: Section['id'], kinds = [] as ReturnType<typeof kind>[]) => ({ id, kinds, items: [] })

describe('section verdict', () => {
  it('reads every section of the mock overview in plain words', () => {
    expect(sectionVerdict(section('context'))).toBe('Agents follow 1 rule and can hand work to 3 agents.')
    expect(sectionVerdict(section('authority'))).toBe('Agents run 11 actions freely, ask before 3, and never do 4.')
    expect(sectionVerdict(section('tools'))).toBe('Agents can use 4 MCP servers.')
    expect(sectionVerdict(section('lifecycle'))).toBe('No hooks run around the agent yet.')
  })

  it('names the first count and skips empty permission lists', () => {
    expect(sectionVerdict({ ...empty('authority'), items: [item([list('permissions.ask', 1), list('permissions.deny', 2)])] })).toBe('Agents ask before 1 action and never do 2.')
    expect(sectionVerdict({ ...empty('authority'), items: [item([list('permissions.deny', 1)])] })).toBe('Agents never do 1 action.')
    expect(sectionVerdict(empty('authority'))).toBe('No permission rules yet, so each tool asks as it normally does.')
  })

  it('says what is missing when a section is empty', () => {
    expect(sectionVerdict(empty('context', [kind('rule', 'Rules', 0)]))).toBe('This project tells agents nothing yet.')
    expect(sectionVerdict(empty('tools', [kind('mcp', 'MCP servers', 0)]))).toBe('Agents get no extra tools from this project yet.')
    expect(sectionVerdict(empty('settings'))).toBe('Your tools run on their own defaults.')
    expect(sectionVerdict(empty('context', [kind('instructions', 'Instructions', 1), kind('skill', 'Skills', 2)]))).toBe('Agents read 1 instructions file and can run 2 skills.')
  })

  it('counts hooks and the stages they run at', () => {
    const hook = (group: string) => ({ ...item([]), kind: 'hook', group })
    expect(sectionVerdict({ ...empty('lifecycle'), items: [hook('pre-tool'), hook('pre-tool'), hook('session-start')] })).toBe('3 hooks run at 2 stages of a session.')
  })
})

describe('section facts', () => {
  it('adds what the verdict leaves out', () => {
    expect(sectionFacts(section('authority'))).toEqual(['2 protected paths'])
    expect(sectionFacts(section('tools'))).toEqual(['1 more server found in your tools'])
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
