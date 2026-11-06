import { describe, expect, it } from 'vitest'
import type { ItemFinding, OverviewView, Section, SectionItem } from '../contract'
import { countFindings, healthFixPrompt, healthGroups, healthPlace, healthSummary } from './health'
import { findingEntryIds, itemEntryId } from './identity'
import { overviewFindings } from './recheck'
import { loadOverviewFixture } from './testing'

const hookError: ItemFinding = { source: 'lint', code: 'LINT001', severity: 'error', message: 'The hook has no command.', path: '.agnostic-ai/hooks/prepare.yaml', fix: 'Add a command.' }
const hookWarning: ItemFinding = { source: 'lint', code: 'LINT002', severity: 'warning', message: 'The hook has no timeout.', path: '.agnostic-ai/hooks/prepare.yaml' }
const skillWarning = (name: string): ItemFinding => ({ source: 'doctor', severity: 'warn', message: `Skill ${name} is not synced.`, path: `.agnostic-ai/skills/${name}/SKILL.md` })
const sectionError: ItemFinding = { source: 'doctor', severity: 'error', message: 'The configuration file cannot be read.' }

const item = (kind: string, name: string, findings: ItemFinding[]): SectionItem => ({ kind, name, label: name, description: '', path: `.agnostic-ai/${kind}s/${name}`, layer: 'project', values: [], findings })
const section = (id: Section['id'], order: number, items: SectionItem[], findings: ItemFinding[] = []): Section => ({ id, label: id.charAt(0).toUpperCase() + id.slice(1), question: '', order, kinds: [], items, support: [], findings })
const view = (sections: Section[]): OverviewView => ({
  schemaVersion: 'overview/v1',
  snapshot: { project: { root: '/tmp/project' } } as OverviewView['snapshot'],
  targets: [], tools: { source: 'default', items: [] },
  sections,
})

const hook = item('hook', 'prepare', [hookError])
const skills = ['review', 'deploy', 'docs'].map((name) => item('skill', name, [skillWarning(name)]))
const scenario = () => view([section('lifecycle', 4, [structuredClone(hook)]), section('context', 1, structuredClone(skills))])

describe('setup health', () => {
  it('counts one hook error and three skill warnings across sections', () => {
    const count = countFindings(scenario())
    expect(count.errors).toBe(1)
    expect(count.warnings).toBe(3)
    expect(healthSummary(count)).toBe('1 error, 3 warnings')
    expect(count.byPlace.map((place) => `${place.section.id}:${place.item?.name ?? '-'}`)).toEqual(['context:review', 'context:deploy', 'context:docs', 'lifecycle:prepare'])
  })

  it('counts section findings and says when there are none', () => {
    const clean = view([section('settings', 5, [item('settings', 'settings', [])])])
    expect(healthSummary(countFindings(clean))).toBe('No findings')
    const count = countFindings(view([section('settings', 5, [], [sectionError, skillWarning('x')])]))
    expect(count).toMatchObject({ errors: 1, warnings: 1 })
    expect(count.byPlace[0].owner).toBeUndefined()
    expect(healthSummary({ errors: 0, warnings: 1 })).toBe('1 warning')
  })

  it('counts the fixture warning written as warn', () => {
    const count = countFindings(loadOverviewFixture())
    expect(count.errors + count.warnings).toBe(overviewFindings(loadOverviewFixture()).length)
    expect(count.warnings).toBeGreaterThan(0)
  })

  it('groups by severity, then section in harness order, and keeps the section page link ids', () => {
    const overview = view([section('lifecycle', 4, [item('hook', 'prepare', [hookWarning, hookError])]), section('context', 1, structuredClone(skills), [sectionError])])
    const groups = healthGroups(overview)
    expect(groups.map((group) => group.severity)).toEqual(['error', 'warning'])
    expect(groups[0].sections.map((s) => s.section.id)).toEqual(['context', 'lifecycle'])
    expect(groups[1].sections.map((s) => s.section.id)).toEqual(['context', 'lifecycle'])
    const owner = itemEntryId(overview.sections[0].items[0])
    const ids = findingEntryIds([hookWarning, hookError], owner)
    const hookError0 = groups[0].sections[1].entries[0]
    expect(hookError0.findings).toEqual([hookError])
    expect(hookError0.ids).toEqual([ids[1]])
    expect(hookError0.entry).toBe(owner)
    expect(hookError0.place).toBe(`health:error:${owner}`)
    expect(groups[1].sections[1].entries[0].ids).toEqual([ids[0]])
    const contextError = groups[0].sections[0].entries[0]
    expect(contextError.item).toBeUndefined()
    expect(contextError.entry).toBe(findingEntryIds([sectionError])[0])
    expect(contextError.place).toBe('health:error:section:context')
  })

  it('keeps the place a recheck started from once its findings are gone', () => {
    const before = scenario()
    const after = scenario()
    after.sections[0].items[0].findings = []
    const place = healthPlace('error', { section: before.sections[0], owner: itemEntryId(hook) })
    expect(healthGroups(after).map((group) => group.severity)).toEqual(['warning'])
    const kept = healthGroups(after, { place, before })
    expect(kept.map((group) => group.severity)).toEqual(['error', 'warning'])
    expect(kept[0].sections[0].entries[0]).toMatchObject({ place, findings: [], ids: [], entry: itemEntryId(hook) })
    expect(kept[0].sections[0].entries[0].item?.name).toBe('prepare')
  })

  it('builds one prompt for every finding, errors first', () => {
    const prompt = healthFixPrompt(scenario(), '/tmp/project')
    expect(prompt).toMatch(/^Fix these 4 agnostic-ai findings in the selected project or user configuration, errors first:/)
    expect(prompt).toContain('Configuration root: /tmp/project')
    expect(prompt.indexOf('1. Lifecycle: prepare')).toBeGreaterThan(0)
    expect(prompt.indexOf('1. Lifecycle: prepare')).toBeLessThan(prompt.indexOf('2. Context: review'))
    expect(prompt).toContain('4. Context: docs')
    expect(prompt).toContain('Check: lint (LINT001)')
    expect(prompt).toContain('Suggested fix: Add a command.')
    expect(prompt.match(/canonical specs, not generated files/g)).toHaveLength(1)
    expect(prompt).not.toContain('undefined')
  })

  it('names a single finding in the singular', () => {
    const one = view([section('lifecycle', 4, [structuredClone(hook)])])
    expect(healthFixPrompt(one)).toMatch(/^Fix this agnostic-ai finding in the selected project or user configuration:\n\n1\. Lifecycle: prepare/)
  })
})
