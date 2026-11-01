import { describe, expect, it } from 'vitest'
import type { ToolCatalog, ToolChoice } from '../contract'
import { filterTools, offConsequence, saveHint, toolCount, toolOn } from './coding-tools'

const tool = (over: Partial<ToolChoice>): ToolChoice => ({ target: 'claude', label: 'Claude Code', description: 'Claude Code (Anthropic)', enabled: true, local: false, detected: false, ...over })

describe('coding tools', () => {
  it('shows the shared state in Shared when a personal list differs', () => {
    const personalOff = tool({ enabled: false, local: true })
    expect(toolOn(personalOff, 'personal')).toBe(false)
    expect(toolOn(personalOff, 'shared')).toBe(true)
    expect(toolOn(tool({}), 'shared')).toBe(true)
  })

  it('filters by name, id, or description and by on state', () => {
    const items = [tool({}), tool({ target: 'cursor', label: 'Cursor', description: 'Cursor (cursor.com)', enabled: false })]
    expect(filterTools(items, 'anthropic', false, 'shared').map((t) => t.target)).toEqual(['claude'])
    expect(filterTools(items, 'CUR', false, 'shared').map((t) => t.target)).toEqual(['cursor'])
    expect(filterTools(items, '', true, 'shared').map((t) => t.target)).toEqual(['claude'])
    expect(toolCount(items, 'shared')).toEqual({ on: 1, total: 2 })
  })

  it('words the consequence and the save hint per mode', () => {
    expect(offConsequence('Cursor', false)).toContain('removes the files it wrote for Cursor')
    expect(offConsequence('Cursor', false)).toContain('running sync --watch')
    expect(offConsequence('Cursor', true)).toContain('stay until you remove them by hand')
    const catalog: ToolCatalog = { source: 'default', items: [] }
    expect(saveHint(catalog, 'shared', false)).toContain('the default list applies')
    expect(saveHint({ ...catalog, source: 'shared' }, 'personal', false)).toContain('agnostic-ai.local.yaml')
    expect(saveHint({ ...catalog, source: 'shared' }, 'shared', true)).toContain('sync --global')
    for (const text of [offConsequence('X', false), offConsequence('X', true), saveHint(catalog, 'shared', false)]) expect(text).not.toMatch(/[–—]/)
  })
})
