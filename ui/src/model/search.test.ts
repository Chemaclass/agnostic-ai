import { findingTitle } from './finding-title'
import { describe, expect, it } from 'vitest'
import type { OverviewView, SectionItem } from '../contract'
import { fieldEntryId, findingEntryIds, itemEntryId } from './identity'
import { buildSearchIndex, itemSearchText, matchesSearch, searchOverview } from './search'
import { loadOverviewFixture } from './testing'

const fixture = loadOverviewFixture('src/fixtures/overview.mock.json')

function search(data: OverviewView, query: string) {
  return searchOverview(buildSearchIndex(data), query)
}

describe('overview search', () => {
  it('finds a server without knowing its section, and links to it', () => {
    const server = fixture.sections.find((section) => section.id === 'tools')!.items.find((item) => item.name === 'github')!
    const [first] = search(fixture, 'github')
    expect(first).toMatchObject({ view: 'tools', kind: 'item', entry: itemEntryId(server), title: server.label })
    expect(first.context).toBe('Tools · MCP servers')
    expect(first.detail).toBe(server.path)
  })

  it('finds settings by label, key, and displayed value, and findings by message and code', () => {
    const permissionMode = fixture.sections.find((section) => section.id === 'authority')!.fields!.find((field) => field.label === 'Default permission mode')!
    expect(search(fixture, 'default permission mode').map((result) => result.entry)).toContain(fieldEntryId(permissionMode))
    expect(search(fixture, 'shell(go test:*)').some((result) => result.view === 'authority')).toBe(true)
    const authority = fixture.sections.find((section) => section.id === 'authority')!
    const owner = authority.items.find((item) => item.findings.length)!
    const finding = owner.findings[0]
    const results = search(fixture, finding.code!)
    expect(results).toContainEqual(expect.objectContaining({ kind: 'finding', view: 'authority', entry: findingEntryIds(owner.findings, itemEntryId(owner))[0] }))
    const result = results.find((r) => r.kind === 'finding')!
    expect(result.title).toBe(findingTitle(finding))
    expect(result.detail).toBe(finding.path)
    expect(search(fixture, finding.message.slice(0, 30))).toContainEqual(expect.objectContaining({ kind: 'finding', entry: result.entry }))
  })

  it('finds a hook by the command it runs', () => {
    const data = structuredClone(fixture)
    const lifecycle = data.sections.find((section) => section.id === 'lifecycle')!
    lifecycle.items.push({ kind: 'hook', name: 'guard', label: 'guard', description: '', path: '.agnostic-ai/hooks/guard.yaml', layer: 'project', group: 'before-tool', values: [], fields: [{ key: 'command', type: 'text', value: './scripts/no-force-push.sh' }], findings: [] })
    expect(search(data, 'no-force-push')).toContainEqual(expect.objectContaining({ view: 'lifecycle', kind: 'item', title: 'guard' }))
  })

  it('requires every word, ignores case, and returns nothing for an empty or unmatched query', () => {
    expect(search(fixture, '   ')).toEqual([])
    expect(search(fixture, 'no-such-thing-anywhere')).toEqual([])
    expect(search(fixture, 'GITHUB mcps').length).toBeGreaterThan(0)
    expect(search(fixture, 'github no-such-word')).toEqual([])
  })

  it('tells apart entries with the same name by section and path', () => {
    const data = structuredClone(fixture)
    const context = data.sections.find((section) => section.id === 'context')!
    context.items.push({ ...context.items[0], path: '~/.agnostic-ai/rules/testing.md', layer: 'global' })
    const twins = search(data, context.items[0].name).filter((result) => result.title === context.items[0].label)
    expect(new Set(twins.map((result) => result.entry)).size).toBe(twins.length)
    expect(new Set(twins.map((result) => `${result.context} ${result.detail}`)).size).toBe(twins.length)
  })
})

describe('section search', () => {
  const item: SectionItem = {
    kind: 'rule', name: 'quality-gate', label: 'Quality checks', path: '.agnostic-ai/rules/policies.md',
    description: 'Run checks before sharing changes.', layer: 'project', findings: [],
    values: [
      { key: 'release-channel', type: 'text', value: 'preview' },
      { key: 'branches', type: 'list', list: ['mainline'] },
    ],
  }

  it.each([
    ['name', 'quality-gate'],
    ['label', 'quality checks'],
    ['path', 'rules/policies.md'],
    ['description, ignoring case', 'SHARING CHANGES'],
    ['value key', 'release-channel'],
    ['text value', 'preview'],
    ['list value', 'mainline'],
    ['blank query', '  '],
  ])('matches an item by its %s', (_, query) => {
    expect(matchesSearch(itemSearchText(item), query)).toBe(true)
  })

  it('does not match an item that holds no part of the query', () => {
    expect(matchesSearch(itemSearchText(item), 'no-such-item')).toBe(false)
  })
})
