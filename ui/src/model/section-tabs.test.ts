import { describe, expect, it } from 'vitest'
import type { Section, SectionItem } from '../contract'
import { fieldEntryId, findingEntryIds, itemEntryId, kindEntryId } from './identity'
import { entryTab, landingTab, sectionTabs, tabTitle } from './section-tabs'

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

  it('puts Tools tabs with content first and empty ones last', () => {
    const env: SectionItem = { kind: 'environment', name: 'setup', label: 'setup', path: 'environments/setup.yaml', layer: 'project', description: '', values: [], findings: [] }
    const kinds = [{ kind: 'mcp', label: 'MCP servers', count: 0 }, { kind: 'environment', label: 'Environments', count: 1 }]
    const native = [{ name: 'paper', holders: [{ target: 'claude', label: 'Claude Code', path: '~/.claude.json', scope: 'user' as const }] }]
    expect(sectionTabs(section({ id: 'tools', kinds, items: [env], native })).map((tab) => tab.id)).toEqual(['native', 'environment', 'servers'])
    expect(sectionTabs(section({ id: 'tools', kinds, items: [env] })).map((tab) => tab.id)).toEqual(['environment', 'servers', 'native'])
  })

  it('finds the tab for a linked item, finding, or field', () => {
    const item: SectionItem = { kind: 'agent', name: 'reviewer', label: 'reviewer', path: 'agents/reviewer.md', layer: 'project', description: '', values: [], findings: [{ source: 'lint', severity: 'warning', code: 'LINT', message: 'Check the source.' }] }
    const context = section({ kinds: [{ kind: 'agent', label: 'Agents', count: 1 }], items: [item] })
    const tabs = sectionTabs(context)
    expect(entryTab(context, tabs, itemEntryId(item))).toBe('agents')
    expect(entryTab(context, tabs, findingEntryIds(item.findings, itemEntryId(item))[0])).toBe('agents')
    const field = { key: 'model', label: 'Model', description: '', set: [] }
    const settings = section({ id: 'settings', kinds: [{ kind: 'settings', label: 'Preferences', count: 1 }], fields: [field] })
    expect(entryTab(settings, sectionTabs(settings), fieldEntryId(field))).toBe('preferences')
    expect(entryTab(settings, sectionTabs(settings), 'unknown')).toBeUndefined()
  })

  it('finds the tab that holds a linked kind', () => {
    const context = section({ kinds: [{ kind: 'instructions', label: 'Instructions', count: 1 }, { kind: 'review', label: 'Review instructions', count: 1 }, { kind: 'command', label: 'Commands', count: 1 }] })
    const tabs = sectionTabs(context)
    expect(entryTab(context, tabs, kindEntryId('review'))).toBe('guidance')
    expect(entryTab(context, tabs, kindEntryId('command'))).toBe('command')
    expect(entryTab(context, tabs, kindEntryId('skill'))).toBeUndefined()
  })

  it('names the kinds a tab holds in its title, only those the section has', () => {
    const context = section({ kinds: [{ kind: 'instructions', label: 'Instructions', count: 1 }, { kind: 'review', label: 'Review instructions', count: 1 }, { kind: 'ignore', label: 'Ignore files', count: 1 }, { kind: 'rule', label: 'Rules', count: 1 }] })
    const [, guidance, rules] = sectionTabs(context)
    expect(tabTitle(context, guidance)).toBe('Instructions, review instructions, ignore files')
    expect(tabTitle(context, rules)).toBe('Rules')
    const partial = section({ kinds: [{ kind: 'instructions', label: 'Instructions', count: 1 }] })
    expect(tabTitle(partial, sectionTabs(partial)[1])).toBe('Instructions')
    const tools = section({ id: 'tools', kinds: [{ kind: 'mcp', label: 'MCP servers', count: 1 }] })
    const [servers, native] = sectionTabs(tools)
    expect(tabTitle(tools, servers)).toBe('MCP servers')
    expect(tabTitle(tools, native)).toBeUndefined()
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

  it('keeps the coding tools and editor tabs in Settings even without items of their own', () => {
    const settings = section({ id: 'settings', kinds: [{ kind: 'builtin', label: 'Built-ins', count: 1 }, { kind: 'config', label: 'Config files', count: 1 }] })
    expect(sectionTabs(settings).map((tab) => tab.id)).toEqual(['builtins', 'targets', 'configuration', 'editor'])
  })
})
