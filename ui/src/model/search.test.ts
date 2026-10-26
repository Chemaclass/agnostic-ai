import { describe, expect, it } from 'vitest'
import type { OverviewView } from '../contract'
import { fieldEntryId, findingEntryIds, itemEntryId } from './identity'
import { buildSearchIndex, searchOverview } from './search'
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
