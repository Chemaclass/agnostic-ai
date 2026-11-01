import { describe, expect, it } from 'vitest'
import type { Section, SectionItem } from '../contract'
import { fieldEntryId, findingEntryIds, itemEntryId } from './identity'
import { entryTab, landingTab, sectionTabs } from './section-tabs'

const section = (over: Partial<Section>): Section => ({ id: 'context', label: 'Context', question: '', order: 0, kinds: [], items: [], findings: [], support: [], ...over })

describe('section tabs', () => {
  it('groups guidance without losing review and ignore sources', () => {
    const data = section({ kinds: ['instructions', 'review', 'ignore', 'rule', 'agent', 'skill'].map((kind) => ({ kind, label: kind, count: 1 })) })
    const tabs = sectionTabs(data)
    expect(tabs.map((tab) => tab.id)).toEqual(['usage', 'guidance', 'rules', 'agents', 'skills'])
    expect(tabs[1].kinds).toEqual(['instructions', 'review', 'ignore'])
  })

  it('offers native servers even when none are listed and keeps one-purpose pages continuous', () => {
    expect(sectionTabs(section({ id: 'tools', kinds: [{ kind: 'mcp', label: 'MCP servers', count: 0 }] })).map((tab) => tab.id)).toEqual(['servers', 'native'])
    expect(sectionTabs(section({ id: 'authority' }))).toEqual([])
    expect(sectionTabs(section({ id: 'lifecycle' }))).toEqual([])
  })

  it('finds the tab for a linked item, finding, or field', () => {
    const item: SectionItem = { kind: 'agent', name: 'reviewer', label: 'reviewer', path: 'agents/reviewer.md', layer: 'project', description: '', values: [], findings: [{ source: 'lint', severity: 'warning', code: 'LINT', message: 'Check the source.' }] }
    const context = section({ kinds: [{ kind: 'agent', label: 'Agents', count: 1 }], items: [item] })
    const tabs = sectionTabs(context)
    expect(entryTab(context, tabs, itemEntryId(item))).toBe('agents')
    expect(entryTab(context, tabs, findingEntryIds(item.findings, itemEntryId(item))[0])).toBe('agents')
    const field = { key: 'model', label: 'Model', description: '', set: [] }
    const settings = section({ id: 'settings', kinds: [{ kind: 'settings', label: 'Other settings', count: 1 }], fields: [field] })
    expect(entryTab(settings, sectionTabs(settings), fieldEntryId(field))).toBe('preferences')
    expect(entryTab(settings, sectionTabs(settings), 'unknown')).toBeUndefined()
  })

  it('lands on Context size only when it can load, else on the first tab with content', () => {
    const rule: SectionItem = { kind: 'rule', name: 'go', label: 'go', path: 'rules/go.md', layer: 'project', description: '', values: [], findings: [] }
    const context = section({ kinds: ['instructions', 'rule', 'skill'].map((kind) => ({ kind, label: kind, count: 0 })), items: [rule] })
    const tabs = sectionTabs(context)
    expect(landingTab(context, tabs, true)).toBe('usage')
    expect(landingTab(context, tabs, false)).toBe('rules')
    const empty = section({ kinds: [{ kind: 'rule', label: 'Rules', count: 0 }] })
    expect(landingTab(empty, sectionTabs(empty), false)).toBe('usage')
    const tools = section({ id: 'tools', kinds: [{ kind: 'mcp', label: 'MCP servers', count: 0 }], native: [{ name: 'notion', holders: [] }] })
    expect(landingTab(tools, sectionTabs(tools), true)).toBe('native')
  })
})
