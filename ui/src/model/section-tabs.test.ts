import { describe, expect, it } from 'vitest'
import type { Section } from '../contract'
import { fieldEntryId, findingEntryIds, itemEntryId } from './identity'
import { entryTab, sectionTabs } from './section-tabs'

const section = (over: Partial<Section>): Section => ({ id: 'context', label: 'Context', question: '', kinds: [], items: [], findings: [], support: [], ...over })

describe('section tabs', () => {
  it('groups guidance without losing review and ignore sources', () => {
    const data = section({ kinds: ['instructions', 'review', 'ignore', 'rule', 'agent', 'skill'].map((kind) => ({ kind, label: kind, count: 1 })) })
    const tabs = sectionTabs(data)
    expect(tabs.map((tab) => tab.id)).toEqual(['guidance', 'rules', 'agents', 'skills', 'usage'])
    expect(tabs[0].kinds).toEqual(['instructions', 'review', 'ignore'])
  })

  it('offers native servers even when none are listed and keeps one-purpose pages continuous', () => {
    expect(sectionTabs(section({ id: 'tools', kinds: [{ kind: 'mcp', label: 'MCP servers', count: 0 }] })).map((tab) => tab.id)).toEqual(['servers', 'native'])
    expect(sectionTabs(section({ id: 'authority' }))).toEqual([])
    expect(sectionTabs(section({ id: 'lifecycle' }))).toEqual([])
  })

  it('finds the tab for a linked item, finding, or field', () => {
    const item = { kind: 'agent', name: 'reviewer', label: 'reviewer', path: 'agents/reviewer.md', description: '', values: [], findings: [{ source: 'lint', severity: 'warning' as const, code: 'LINT', message: 'Check the source.' }] }
    const context = section({ kinds: [{ kind: 'agent', label: 'Agents', count: 1 }], items: [item] })
    const tabs = sectionTabs(context)
    expect(entryTab(context, tabs, itemEntryId(item))).toBe('agents')
    expect(entryTab(context, tabs, findingEntryIds(item.findings, itemEntryId(item))[0])).toBe('agents')
    const field = { key: 'model', label: 'Model', description: '', set: [] }
    const settings = section({ id: 'settings', kinds: [{ kind: 'settings', label: 'Other settings', count: 1 }], fields: [field] })
    expect(entryTab(settings, sectionTabs(settings), fieldEntryId(field))).toBe('preferences')
    expect(entryTab(settings, sectionTabs(settings), 'unknown')).toBeUndefined()
  })
})
